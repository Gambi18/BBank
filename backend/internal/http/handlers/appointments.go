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

	"github.com/go-chi/chi/v5"
)

type AppointmentHandler struct {
	svc *service.AppointmentService
}

func NewAppointmentHandler(svc *service.AppointmentService) *AppointmentHandler {
	return &AppointmentHandler{svc: svc}
}

func (h *AppointmentHandler) Routes() chi.Router {
	r := chi.NewRouter()
	read := middleware.RequirePermission("appointments", domain.Read)
	r.With(read).Get("/", h.list)
	r.With(read).Get("/{id}", h.get)
	return r
}

func (h *AppointmentHandler) list(w http.ResponseWriter, r *http.Request) {
	scope, ok := resolveScope(r)
	if !ok {
		response.Paged(w, []dto.Appointment{}, 0, response.DefaultLimit, 0)
		return
	}
	paging, ok := response.ParsePaging(w, r)
	if !ok {
		return
	}

	p := service.ListAppointmentParams{Scope: scope, Limit: paging.Limit, Offset: paging.Offset}

	// `?donor_id=` is a convenience filter for callers already scoped wider than
	// one donor. It is ANDed with the scope, so it can narrow a result set and
	// never widen one. A donor's own scope ignores it entirely — that asymmetry
	// is defect A14, fixed as a shape rather than as a check.
	if middleware.ScopeFrom(r.Context()) != domain.ScopeOwn {
		if v := r.URL.Query().Get("donor_id"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				response.BadRequest(w, r, "donor_id must be an integer",
					response.Detail{Field: "donor_id", Issue: "not an integer"})
				return
			}
			p.DonorFilter = &n
		}
	}

	rows, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		response.Internal(w, r, err)
		return
	}
	out := make([]dto.Appointment, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.Appointment{
			ID:              int64(row.ID),
			RequestID:       row.DonationRequestID,
			DonorID:         int64(row.DonorID),
			DonorName:       row.DonorName,
			CenterID:        row.CenterID,
			Status:          string(row.Status),
			ScheduledAt:     tsStr(row.ScheduledAt),
			ScheduledDate:   dateStr(row.ScheduledDate),
			AppointmentDate: dateStr(row.ScheduledDate),
			CheckedInAt:     tsPtr(row.CheckedInAt),
			CompletedAt:     tsPtr(row.CompletedAt),
			CancelledAt:     tsPtr(row.CancelledAt),
		})
	}
	response.Paged(w, out, total, paging.Limit, paging.Offset)
}

func (h *AppointmentHandler) get(w http.ResponseWriter, r *http.Request) {
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

	// WI-02 -> WI-20 — the authorization bypass, closed properly.
	//
	// WI-02 made the ownership check unconditional but still compared against
	// `?donor_id=`, a value the caller supplied: a bypass fix, not
	// authorization. The comparison is against the `sub` claim of a verified
	// token, so asserting someone else's identity is not something a request can
	// express. `?donor_id=` is not read here at all.
	//
	// 404, not 403: a non-owner must not learn that this appointment exists.
	if !middleware.Permits(r.Context(), middleware.Row{OwnerID: int64(row.DonorID), CenterID: &row.CenterID}) {
		middleware.Deny(w)
		return
	}

	response.OK(w, dto.Appointment{
		ID:              int64(row.ID),
		RequestID:       row.DonationRequestID,
		DonorID:         int64(row.DonorID),
		DonorName:       row.DonorName,
		CenterID:        row.CenterID,
		Status:          string(row.Status),
		ScheduledAt:     tsStr(row.ScheduledAt),
		ScheduledDate:   dateStr(row.ScheduledDate),
		AppointmentDate: dateStr(row.ScheduledDate),
		CheckedInAt:     tsPtr(row.CheckedInAt),
		CompletedAt:     tsPtr(row.CompletedAt),
		CancelledAt:     tsPtr(row.CancelledAt),
	})
}
