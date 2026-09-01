// Package handlers holds HTTP handlers: decode -> validate -> call service ->
// encode. No SQL, no business rules.
package handlers

import (
	"errors"
	"net/http"
	"strconv"

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
	return r
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
	if middleware.Permits(r.Context(), middleware.Row{OwnerID: requested}) {
		return true
	}
	response.NotFound(w, r)
	return false
}

func (h *DonorHandler) list(w http.ResponseWriter, r *http.Request) {
	p := service.ListParams{}
	if s := r.URL.Query().Get("search"); s != "" {
		p.Search = &s
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			response.BadRequest(w, r, "limit must be an integer")
			return
		}
		p.Limit = int32(n)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			response.BadRequest(w, r, "offset must be an integer")
			return
		}
		p.Offset = int32(n)
	}

	// A donor's "own" scope makes a full listing meaningless; narrow it to self.
	if middleware.ScopeFrom(r.Context()) == domain.ScopeOwn {
		if id, ok := middleware.IdentityFrom(r.Context()); ok {
			row, err := h.svc.Get(r.Context(), id.UserID)
			if errors.Is(err, service.ErrNotFound) {
				// A user with no donor profile is an empty list, not a 500.
				response.Paged(w, []dto.DonorSummary{}, 0, 1, 0)
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
			}}, 1, 1, 0)
			return
		}
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
	// normalise() clamps these, so echo back what was actually used.
	limit, offset := p.Limit, p.Offset
	if limit == 0 {
		limit = 25
	}
	response.Paged(w, out, total, limit, offset)
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
