package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"bbank/internal/domain"
	"bbank/internal/http/dto"
	"bbank/internal/http/response"
	"bbank/internal/middleware"
	"bbank/internal/service"
	"bbank/internal/store"

	"github.com/go-chi/chi/v5"
)

// DonationRequestHandler serves /api/v1/donation-requests (WI-22).
//
// This replaces the hand-written SQL handlers in internal/legacy. The
// authorization rules are unchanged — they were already correct after WI-20 —
// but they are now expressed as parameters rather than as concatenated SQL, and
// the decide-and-write steps happen inside one locked transaction.
type DonationRequestHandler struct {
	svc  *service.DonationRequestService
	idem middleware.IdempotencyStore
}

func NewDonationRequestHandler(svc *service.DonationRequestService, idem middleware.IdempotencyStore) *DonationRequestHandler {
	return &DonationRequestHandler{svc: svc, idem: idem}
}

func (h *DonationRequestHandler) Routes() chi.Router {
	r := chi.NewRouter()
	replay := middleware.Idempotency(h.idem, false)

	rq := func(a domain.Action) func(http.Handler) http.Handler {
		return middleware.RequirePermission("donation_requests", a)
	}

	r.With(rq(domain.Read)).Get("/", h.list)
	r.With(rq(domain.Create), replay).Post("/", h.create)
	// The controlled rejection vocabulary is served so a UI renders the list
	// from the API rather than keeping its own copy, which drifts.
	r.With(rq(domain.Read)).Get("/rejection-reasons", h.rejectionReasons)
	r.With(rq(domain.Read)).Get("/{id}", h.get)

	// Approving and rejecting are `X-approve` / `X-reject` — staff and admin
	// hold them, a donor does not, even on their own request (§7.6). A bare
	// Execute check would let a donor approve themselves and delete the review
	// step the system exists to perform.
	r.With(middleware.RequireTransition("donation_requests", "approve"), replay).Post("/{id}/approve", h.approve)
	r.With(middleware.RequireTransition("donation_requests", "reject"), replay).Post("/{id}/reject", h.reject)
	// Cancel is the donor's own withdrawal, so donors hold it too.
	r.With(middleware.RequireTransition("donation_requests", "cancel"), replay).Post("/{id}/cancel", h.cancel)
	return r
}

func (h *DonationRequestHandler) list(w http.ResponseWriter, r *http.Request) {
	scope, ok := resolveScope(r)
	if !ok {
		response.Paged(w, []dto.DonationRequest{}, 0, response.DefaultLimit, 0)
		return
	}
	paging, ok := response.ParsePaging(w, r)
	if !ok {
		return
	}

	p := service.ListRequestParams{Scope: scope, Limit: paging.Limit, Offset: paging.Offset}
	if s := r.URL.Query().Get("status"); s != "" {
		if !isRequestStatus(s) {
			response.BadRequest(w, r, "unknown status",
				response.Detail{Field: "status", Issue: "not a donation request status"})
			return
		}
		p.Status = &s
	}

	rows, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		response.Internal(w, r, err)
		return
	}
	out := make([]dto.DonationRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.DonationRequest{
			ID:              int64(row.ID),
			DonorID:         int64(row.DonorID),
			DonorName:       row.DonorName,
			CenterID:        row.CenterID,
			Status:          string(row.Status),
			PreferredDate:   dateStr(row.PreferredDate),
			CreatedAt:       tsStr(row.CreatedAt),
			ReviewedAt:      tsPtr(row.ReviewedAt),
			RejectionReason: row.RejectionReason,
			Notes:           row.Notes,
			LastDonation:    dateStr(row.LegacyLastDonation),
		})
	}
	response.Paged(w, out, total, paging.Limit, paging.Offset)
}

func (h *DonationRequestHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam32(w, r)
	if !ok {
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
	// 404, not 403 — a caller outside the scope must not learn the row exists.
	if !middleware.Permits(r.Context(), middleware.Row{OwnerID: int64(row.DonorID), CenterID: &row.CenterID}) {
		middleware.Deny(w)
		return
	}
	response.OK(w, dto.DonationRequest{
		ID:              int64(row.ID),
		DonorID:         int64(row.DonorID),
		DonorName:       row.DonorName,
		CenterID:        row.CenterID,
		Status:          string(row.Status),
		PreferredDate:   dateStr(row.PreferredDate),
		CreatedAt:       tsStr(row.CreatedAt),
		ReviewedAt:      tsPtr(row.ReviewedAt),
		RejectionReason: row.RejectionReason,
		Notes:           row.Notes,
		LastDonation:    dateStr(row.LegacyLastDonation),
	})
}

func (h *DonationRequestHandler) create(w http.ResponseWriter, r *http.Request) {
	var in dto.CreateDonationRequest
	if err := decodeOptional(r, &in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}

	caller, _ := middleware.IdentityFrom(r.Context())
	p := service.CreateRequestParams{DonorID: caller.UserID, Notes: in.Notes, CenterID: in.CenterID}

	// A donor raises a request for themselves and for nobody else, whatever the
	// body says. Staff and admin may raise one on a donor's behalf, so their
	// wider scope reads donor_id from the payload.
	if middleware.ScopeFrom(r.Context()) != domain.ScopeOwn && in.DonorID != nil {
		p.DonorID = *in.DonorID
	}
	if in.PreferredDate != nil && *in.PreferredDate != "" {
		d, err := time.Parse("2006-01-02", *in.PreferredDate)
		if err != nil {
			response.Unprocessable(w, r, "preferred_date must be YYYY-MM-DD",
				response.Detail{Field: "preferred_date", Issue: "not a date"})
			return
		}
		p.PreferredDate = &d
	}
	if in.Procedure != nil {
		proc, err := domain.ParseProcedure(*in.Procedure)
		if err != nil {
			response.Unprocessable(w, r, err.Error(),
				response.Detail{Field: "procedure", Issue: "not a known donation procedure"})
			return
		}
		p.Procedure = proc
	}

	// The override is built from the CALLER'S OWN role, read from the verified
	// token — never from anything in the body. A request that could name its own
	// role would be a request that could grant itself one, which is the shape of
	// the A14 defect this codebase has already fixed twice.
	if in.OverridePermanentDeferralReason != nil {
		p.Override = &service.PermanentDeferralOverride{
			ActorID:   caller.UserID,
			ActorRole: domain.Role(caller.Role),
			Reason:    *in.OverridePermanentDeferralReason,
		}
	}

	row, err := h.svc.Create(r.Context(), p)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.Created(w, dto.DonationRequest{
		ID:            int64(row.ID),
		DonorID:       int64(row.DonorID),
		CenterID:      row.CenterID,
		Status:        string(row.Status),
		PreferredDate: dateStr(row.PreferredDate),
		CreatedAt:     tsStr(row.CreatedAt),
	})
}

// approve sets status='approved' and creates the appointment in one
// transaction. **It does not delete the request row** — the original `confirm`
// did, and that destroyed the link back to who asked and when (A8 / TD-05).
func (h *DonationRequestHandler) approve(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam32(w, r)
	if !ok {
		return
	}
	var in dto.ApproveDonationRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}
	if in.Date == "" {
		response.Unprocessable(w, r, "a date is required",
			response.Detail{Field: "date", Issue: "required"})
		return
	}
	date, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		response.Unprocessable(w, r, "date must be YYYY-MM-DD",
			response.Detail{Field: "date", Issue: "not a date"})
		return
	}

	caller, _ := middleware.IdentityFrom(r.Context())
	appt, err := h.svc.Approve(r.Context(), id, caller.UserID, date, permitFunc(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.Created(w, dto.Appointment{
		ID:            int64(appt.ID),
		RequestID:     deref32(appt.DonationRequestID),
		DonorID:       int64(appt.DonorID),
		CenterID:      appt.CenterID,
		Status:        string(appt.Status),
		ScheduledAt:   tsStr(appt.ScheduledAt),
		ScheduledDate: in.Date,
		// Legacy field name the current frontend still reads.
		AppointmentDate: in.Date,
	})
}

func (h *DonationRequestHandler) reject(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam32(w, r)
	if !ok {
		return
	}
	var in dto.RejectDonationRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}

	caller, _ := middleware.IdentityFrom(r.Context())
	err := h.svc.Reject(r.Context(), id, caller.UserID, domain.RejectionReason(in.Reason), in.Note, permitFunc(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *DonationRequestHandler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam32(w, r)
	if !ok {
		return
	}
	caller, _ := middleware.IdentityFrom(r.Context())
	if err := h.svc.Cancel(r.Context(), id, caller.UserID, permitFunc(r)); err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.NoContent(w)
}

func (h *DonationRequestHandler) rejectionReasons(w http.ResponseWriter, _ *http.Request) {
	reasons := domain.RejectionReasons()
	out := make([]dto.RejectionReason, 0, len(reasons))
	for _, x := range reasons {
		out = append(out, dto.RejectionReason{Value: string(x.Value), Label: x.Label})
	}
	response.OK(w, out)
}

func isRequestStatus(s string) bool {
	switch store.DonationRequestStatus(s) {
	case store.DonationRequestStatusPending, store.DonationRequestStatusApproved,
		store.DonationRequestStatusRejected, store.DonationRequestStatusCancelled,
		store.DonationRequestStatusExpired:
		return true
	}
	return false
}
