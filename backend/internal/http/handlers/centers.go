package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"bbank/internal/domain"
	"bbank/internal/http/dto"
	"bbank/internal/http/response"
	"bbank/internal/middleware"
	"bbank/internal/service"

	"github.com/go-chi/chi/v5"
)

// CenterHandler serves /api/v1/centers (WI-24, TRD §6.5).
type CenterHandler struct {
	svc  *service.CenterService
	idem middleware.IdempotencyStore
}

func NewCenterHandler(svc *service.CenterService, idem middleware.IdempotencyStore) *CenterHandler {
	return &CenterHandler{svc: svc, idem: idem}
}

// Routes serves /api/v1/centers.
//
// **`GET /` is anonymous**, which TRD §6.5 marks `pub`: the directory is no PHI
// — the same information a centre puts on a poster — and a donor deciding
// whether to sign up needs to know where they would go. It is served at the
// canonical path rather than a separate `/public/…` one, because the TRD names
// that path and a second spelling for one resource is how two of them drift.
//
// Everything else is gated by the §7.6 matrix. `Authenticate` runs globally and
// only POPULATES identity, so a route without `RequirePermission` is public by
// construction rather than by an exemption list somebody has to maintain.
func (h *CenterHandler) Routes() chi.Router {
	r := chi.NewRouter()
	replay := middleware.Idempotency(h.idem, false)

	rq := func(a domain.Action) func(http.Handler) http.Handler {
		return middleware.RequirePermission("donation_centers", a)
	}

	r.Get("/", h.list)
	r.With(rq(domain.Read)).Get("/{id}", h.get)
	// Bookable slots are `auth`, not public: how full a centre is on Tuesday is
	// operational detail, and publishing it invites a scraper to map throughput.
	r.With(rq(domain.Read)).Get("/{id}/slots", h.slots)
	r.With(rq(domain.Create), replay).Post("/", h.create)
	r.With(rq(domain.Update), replay).Patch("/{id}", h.patch)
	return r
}

// list is the directory, and it is deliberately the same handler for everybody.
//
// Active centres only, unless an AUTHENTICATED caller asks for the rest. A
// closed centre is not somewhere to go, so listing it by default would send
// people to a locked door — but an admin managing the estate has to be able to
// see the ones they closed, or reopening becomes impossible through the API.
func (h *CenterHandler) list(w http.ResponseWriter, r *http.Request) {
	includeInactive := false
	if _, authenticated := middleware.IdentityFrom(r.Context()); authenticated {
		includeInactive = r.URL.Query().Get("include_inactive") == "true"
	}
	h.listWith(w, r, !includeInactive)
}

func (h *CenterHandler) listWith(w http.ResponseWriter, r *http.Request, activeOnly bool) {
	paging, ok := response.ParsePaging(w, r)
	if !ok {
		return
	}
	p := service.ListCenterParams{ActiveOnly: activeOnly, Limit: paging.Limit, Offset: paging.Offset}
	if region := r.URL.Query().Get("region"); region != "" {
		p.Region = &region
	}

	rows, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	out := make([]dto.Center, 0, len(rows))
	for _, c := range rows {
		out = append(out, dto.Center{
			ID: c.ID, Code: c.Code, Name: c.Name,
			AddressLine: c.AddressLine, City: c.City, Region: c.Region,
			Phone: c.Phone, Email: c.Email,
			CapacityPerSlot: int(c.CapacityPerSlot), SlotMinutes: int(c.SlotMinutes),
			OpeningHours: json.RawMessage(c.OpeningHours), Timezone: c.Timezone,
			IsActive: c.IsActive,
		})
	}
	response.Paged(w, out, total, paging.Limit, paging.Offset)
}

func (h *CenterHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.OK(w, dto.Center{
		ID: c.ID, Code: c.Code, Name: c.Name,
		AddressLine: c.AddressLine, City: c.City, Region: c.Region,
		Phone: c.Phone, Email: c.Email,
		CapacityPerSlot: int(c.CapacityPerSlot), SlotMinutes: int(c.SlotMinutes),
		OpeningHours: json.RawMessage(c.OpeningHours), Timezone: c.Timezone,
		IsActive: c.IsActive,
	})
}

// slots answers "when can somebody book here", capacity minus what is taken.
func (h *CenterHandler) slots(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}

	day := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		d, err := time.Parse("2006-01-02", v)
		if err != nil {
			response.Unprocessable(w, r, "date must be YYYY-MM-DD",
				response.Detail{Field: "date", Issue: "not a date"})
			return
		}
		day = d
	}

	booked, sched, err := h.svc.SlotOccupancy(r.Context(), id, day)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	taken := make(map[time.Time]int, len(booked))
	for _, b := range booked {
		taken[b.ScheduledAt.Time.In(sched.Location)] = int(b.Booked)
	}

	slots := sched.SlotsOn(day)
	out := make([]dto.Slot, 0, len(slots))
	for _, s := range slots {
		used := taken[s]
		out = append(out, dto.Slot{
			StartsAt:  s.Format(time.RFC3339),
			Capacity:  sched.CapacityPer,
			Booked:    used,
			Available: max(0, sched.CapacityPer-used),
		})
	}
	// An empty list is a real answer — the centre is shut that day — and must
	// not be confused with "we could not tell you", which is why it is a 200
	// with no slots rather than a 404.
	response.OK(w, out)
}

func (h *CenterHandler) create(w http.ResponseWriter, r *http.Request) {
	var in dto.WriteCenter
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}

	row, err := h.svc.Create(r.Context(), centerInput(in))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.Created(w, dto.Center{
		ID: row.ID, Code: row.Code, Name: row.Name,
		AddressLine: row.AddressLine, City: row.City, Region: row.Region,
		Phone: row.Phone, Email: row.Email,
		CapacityPerSlot: int(row.CapacityPerSlot), SlotMinutes: int(row.SlotMinutes),
		OpeningHours: json.RawMessage(row.OpeningHours), Timezone: row.Timezone,
		IsActive: row.IsActive,
	})
}

func (h *CenterHandler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in dto.WriteCenter
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}

	row, err := h.svc.Update(r.Context(), id, centerInput(in))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.OK(w, dto.Center{
		ID: row.ID, Code: row.Code, Name: row.Name,
		AddressLine: row.AddressLine, City: row.City, Region: row.Region,
		Phone: row.Phone, Email: row.Email,
		CapacityPerSlot: int(row.CapacityPerSlot), SlotMinutes: int(row.SlotMinutes),
		OpeningHours: json.RawMessage(row.OpeningHours), Timezone: row.Timezone,
		IsActive: row.IsActive,
	})
}

// centerInput maps the wire shape onto the service's.
//
// Here rather than as a method on the DTO, so `internal/http/dto` stays a
// package of plain shapes with no dependency on `internal/service` — the wire
// format and the service's input are allowed to diverge, and a method on the DTO
// would quietly couple them.
//
// An absent string stays absent: the update query COALESCEs a nil to the stored
// value, which is what PATCH means. Sending `""` is the same as sending nothing,
// deliberately — a centre with an empty name is not something a form should be
// able to produce by accident, and clearing a required field has no meaning.
func centerInput(in dto.WriteCenter) service.CenterInput {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return service.CenterInput{
		Code:            str(in.Code),
		Name:            str(in.Name),
		AddressLine:     str(in.AddressLine),
		City:            str(in.City),
		Region:          str(in.Region),
		Phone:           in.Phone,
		Email:           in.Email,
		CapacityPerSlot: in.CapacityPerSlot,
		SlotMinutes:     in.SlotMinutes,
		OpeningHours:    in.OpeningHours,
		Timezone:        in.Timezone,
		IsActive:        in.IsActive,
	}
}
