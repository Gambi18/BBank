// Package handlers holds HTTP handlers: decode -> validate -> call service ->
// encode. No SQL, no business rules.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bbank/internal/domain"
	"bbank/internal/http/dto"
	"bbank/internal/http/response"
	"bbank/internal/middleware"
	"bbank/internal/service"
	"bbank/internal/store"

	"github.com/go-chi/chi/v5"
)

type DonorHandler struct {
	svc *service.DonorService
}

func NewDonorHandler(svc *service.DonorService) *DonorHandler { return &DonorHandler{svc: svc} }

func (h *DonorHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequireAuth)
	r.With(middleware.RequirePermission("donor_profiles", domain.Read)).Get("/", h.list)
	r.With(middleware.RequirePermission("donor_profiles", domain.Read)).Get("/{id}", h.get)
	r.With(middleware.RequirePermission("donor_profiles", domain.Read)).Get("/{id}/eligibility", h.eligibility)
	r.With(middleware.RequirePermission("donor_profiles", domain.Update)).Patch("/{id}", h.update)
	return r
}

// PublicRoutes are the donor endpoints reachable WITHOUT a session.
//
// Only self-registration (TRD §6.5: `pub`). It is mounted separately rather
// than being carved out of Routes with an exception, because an exception
// inside an authenticated router is the kind of thing that gets copied to the
// next endpoint by accident. A separate router makes the public surface a list
// somebody can read.
func (h *DonorHandler) PublicRoutes(idem middleware.IdempotencyStore) chi.Router {
	r := chi.NewRouter()
	r.With(middleware.Idempotency(idem, false)).Post("/", h.create)
	return r
}

// create registers a donor.
//
// Self-registration is public, so `allowClinical` is false: blood group and
// rhesus are laboratory results (FR-21), and a value someone types about their
// own blood is not evidence. Staff and admin creating a donor on the desk may
// set them, which is decided from the verified token, never from the body.
func (h *DonorHandler) create(w http.ResponseWriter, r *http.Request) {
	var in dto.CreateDonor
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}

	var details []response.Detail
	if strings.TrimSpace(in.Email) == "" {
		details = append(details, response.Detail{Field: "email", Issue: "required"})
	}
	if strings.TrimSpace(in.FullName) == "" {
		details = append(details, response.Detail{Field: "full_name", Issue: "required"})
	}
	if in.Password == "" {
		details = append(details, response.Detail{Field: "password", Issue: "required"})
	}
	if len(details) > 0 {
		response.Unprocessable(w, r, "some required details are missing", details...)
		return
	}

	p := service.CreateParams{
		Email: in.Email, Password: in.Password, FullName: in.FullName,
		Gender: in.Gender, Phone: in.Phone, Address: in.Address,
		BloodGroup: in.BloodGroup, Rhesus: in.Rhesus,
	}
	if in.DateOfBirth != "" {
		dob, err := time.Parse("2006-01-02", in.DateOfBirth)
		if err != nil {
			response.Unprocessable(w, r, "date_of_birth must be YYYY-MM-DD",
				response.Detail{Field: "date_of_birth", Issue: "not a date"})
			return
		}
		p.DateOfBirth = dob
	}
	// Clinical fields only for a caller the matrix grants wider than "own".
	allowClinical := false
	if id, ok := middleware.IdentityFrom(r.Context()); ok {
		allowClinical = id.Role == domain.RoleStaff || id.Role == domain.RoleAdmin
	}

	newID, err := h.svc.Create(r.Context(), p, allowClinical)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/donors/"+strconv.FormatInt(newID, 10))
	response.Created(w, map[string]int64{"id": newID})
}

// update edits a donor profile. A donor may edit their own; admin may edit any.
// Ownership comes from the token via resolveOwned, never from the URL alone.
func (h *DonorHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if !resolveOwned(w, r, id) {
		return
	}
	var in dto.UpdateDonor
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}

	p := service.UpdateParams{
		FullName: in.FullName, Gender: in.Gender, Phone: in.Phone,
		Address: in.Address, City: in.City, Region: in.Region,
		NationalID:           in.NationalID,
		EmergencyContactName: in.EmergencyContactName, EmergencyContactPhone: in.EmergencyContactPhone,
		BloodGroup: in.BloodGroup, Rhesus: in.Rhesus,
	}
	if in.DateOfBirth != "" {
		dob, err := time.Parse("2006-01-02", in.DateOfBirth)
		if err != nil {
			response.Unprocessable(w, r, "date_of_birth must be YYYY-MM-DD",
				response.Detail{Field: "date_of_birth", Issue: "not a date"})
			return
		}
		p.DateOfBirth = dob
	}
	caller, _ := middleware.IdentityFrom(r.Context())
	allowClinical := caller.Role == domain.RoleStaff || caller.Role == domain.RoleAdmin

	if err := h.svc.Update(r.Context(), id, p, allowClinical); err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.NoContent(w)
}

// resolveOwned enforces TRD §7.7: the donor id in a URL is NOT identity.
//
// If the caller's granted scope is "own", the requested id must equal the id in
// their token. A mismatch returns 404, not 403 — a 403 would confirm that the
// other donor exists.
//
// This is the general form of the WI-02 hotfix. There, ownership was compared
// against a query parameter the caller supplied; here it comes from a verified
// token and cannot be influenced by the request at all.
func resolveOwned(w http.ResponseWriter, r *http.Request, requested int64) bool {
	if donorScopePermits(r, requested) {
		return true
	}
	response.NotFound(w, r)
	return false
}

// donorScopePermits answers what the granted scope means for a DONOR record.
//
// `donor_profiles` has no centre column, and deliberately so: a donor may
// attend any centre, and pinning them to one would make the person who moves
// house invisible to the desk they walk up to. That leaves §7.6's `ctr` grant
// for staff with nothing to compare against — `Permits` needs a `row.CenterID`
// and there is none — so this resource has to say what `ctr` means, explicitly,
// in one place.
//
// It means **the whole registry**, per TRD §6.5, which grants staff
// `GET /api/v1/donors` as registry search with `center_id` as an optional
// FILTER rather than a boundary. Check-in (`WI-39`) needs the same: a donor
// walking in may never have attended this centre before.
//
// This was previously guessed twice, differently: `list` fell through to the
// entire registry while `resolveOwned` asked `Permits` for a row with a nil
// centre — which `ScopeCenter` always rejects — so staff could see every donor
// at once and none of them individually. The disagreement is the bug; the
// direction is TRD §6.5's.
func donorScopePermits(r *http.Request, requested int64) bool {
	switch middleware.ScopeFrom(r.Context()) {
	case domain.ScopeAll:
		return true
	case domain.ScopeCenter:
		// See above: registry-wide for donors, because there is no centre to
		// scope to. Revisit if WI-24 ever gives donors a home centre.
		return true
	case domain.ScopeOwn:
		id, ok := middleware.IdentityFrom(r.Context())
		return ok && id.UserID == requested
	default:
		// Aggregate, hospital, or a scope added later: deny until somebody
		// decides what it should mean here.
		return false
	}
}

func (h *DonorHandler) list(w http.ResponseWriter, r *http.Request) {
	// Parsed and CLAMPED here, so the limit reported back is the limit applied.
	// This handler used to echo the requested value while the service quietly
	// clamped a copy of it: ?limit=5000 returned 100 rows and announced 5000, so
	// a caller's paging loop advanced by 5000 and skipped 4,900 records without
	// any error to notice. (WI-21)
	paging, ok := response.ParsePaging(w, r)
	if !ok {
		return
	}
	p := service.ListParams{Limit: paging.Limit, Offset: paging.Offset}
	if s := r.URL.Query().Get("search"); s != "" {
		p.Search = &s
	}

	// The same scope decision as donorScopePermits, applied to a list rather
	// than to one row. Anything that cannot see the whole registry sees only
	// itself, and an unrecognised scope sees nothing.
	switch middleware.ScopeFrom(r.Context()) {
	case domain.ScopeAll, domain.ScopeCenter:
		// Registry search. `?center_id=` is a filter, not a boundary (§6.5).
	case domain.ScopeOwn:
		if id, ok := middleware.IdentityFrom(r.Context()); ok {
			row, err := h.svc.Get(r.Context(), id.UserID)
			if errors.Is(err, service.ErrNotFound) {
				// A user with no donor profile is an empty list, not a 500.
				response.Paged(w, []dto.DonorSummary{}, 0, paging.Limit, paging.Offset)
				return
			}
			if err != nil {
				response.Internal(w, r, err)
				return
			}
			response.Paged(w, []dto.DonorSummary{{
				ID: row.ID, Email: row.Email, FullName: row.FullName,
				BloodGroup: enumPtr(row.BloodGroup), Rhesus: enumPtr(row.Rhesus),
				ContactPhone: row.ContactPhone, TotalDonations: row.TotalDonations,
				Status: string(row.Status),
			}}, 1, paging.Limit, paging.Offset)
			return
		}
		// Own scope with no identity should be unreachable — RequirePermission
		// rejects anonymous callers first — but falling through from here would
		// hand back the whole registry, so it returns empty instead.
		response.Paged(w, []dto.DonorSummary{}, 0, paging.Limit, paging.Offset)
		return
	default:
		// Aggregate, hospital, or a scope added later: nothing, until somebody
		// decides what it should mean. Default-deny, so a new scope cannot
		// inherit registry-wide access by omission.
		response.Paged(w, []dto.DonorSummary{}, 0, paging.Limit, paging.Offset)
		return
	}

	rows, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		response.Internal(w, r, err)
		return
	}

	out := make([]dto.DonorSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.DonorSummary{
			ID:             row.ID,
			Email:          row.Email,
			FullName:       row.FullName,
			BloodGroup:     enumPtr(row.BloodGroup),
			Rhesus:         enumPtr(row.Rhesus),
			ContactPhone:   row.ContactPhone,
			TotalDonations: row.TotalDonations,
			Status:         string(row.Status),
		})
	}
	response.Paged(w, out, total, paging.Limit, paging.Offset)
}

func (h *DonorHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if !resolveOwned(w, r, id) {
		return
	}
	row, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		response.NotFound(w, r)
		return
	}
	if err != nil {
		response.Internal(w, r, err)
		return
	}
	response.OK(w, dto.Donor{
		ID:                    row.ID,
		Email:                 row.Email,
		FullName:              row.FullName,
		DateOfBirth:           datePtr(row.DateOfBirth),
		Gender:                strPtr(string(row.Gender)),
		BloodGroup:            enumPtr(row.BloodGroup),
		Rhesus:                enumPtr(row.Rhesus),
		ContactPhone:          row.ContactPhone,
		AddressLine:           row.AddressLine,
		City:                  row.City,
		Region:                row.Region,
		NationalID:            row.NationalID,
		EmergencyContactName:  row.EmergencyContactName,
		EmergencyContactPhone: row.EmergencyContactPhone,
		TotalDonations:        row.TotalDonations,
		Status:                string(row.Status),
	})
}

func (h *DonorHandler) eligibility(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if !resolveOwned(w, r, id) {
		return
	}
	row, err := h.svc.Eligibility(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		response.NotFound(w, r)
		return
	}
	if err != nil {
		response.Internal(w, r, err)
		return
	}
	response.OK(w, dto.Eligibility{
		DonorID:             row.DonorID,
		FullName:            row.FullName,
		AgeYears:            &row.AgeYears,
		LastDonatedAt:       anyTimePtr(row.LastDonatedAt),
		DonationsLast12m:    derefInt64(row.DonationsLast12m),
		PermanentlyDeferred: derefBool(row.PermanentlyDeferred),
		DeferredUntil:       anyDatePtr(row.DeferredUntil),
		NextEligibleOn:      anyDatePtr(row.NextEligibleOn),
		IsEligibleToday:     derefBool(row.IsEligibleToday),
		Reason:              row.Reason,
	})
}

func idParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(w, r, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func enumPtr[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var _ = store.Queries{} // keep the import meaningful if handlers grow
