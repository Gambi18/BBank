package service

import (
	"context"
	"errors"
	"fmt"

	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
)

type AppointmentService struct {
	q *store.Queries
}

func NewAppointmentService(q *store.Queries) *AppointmentService {
	return &AppointmentService{q: q}
}

type ListAppointmentParams struct {
	Scope Scope
	// DonorFilter is a convenience narrowing for callers already scoped wider
	// than one donor. It is a separate field from Scope.OwnerID precisely so it
	// cannot be mistaken for identity: Scope comes from the token, this comes
	// from the query string, and both are ANDed. A filter can narrow a result
	// set; it can never widen one. That distinction is the A14 defect, stated
	// as a type rather than as a comment.
	DonorFilter *int64
	Limit       int32
	Offset      int32
}

func (s *AppointmentService) List(ctx context.Context, p ListAppointmentParams) ([]store.ListAppointmentsRow, int64, error) {
	rows, err := s.q.ListAppointments(ctx, store.ListAppointmentsParams{
		OwnerID: p.Scope.OwnerID, CenterID: p.Scope.CenterID, DonorFilter: p.DonorFilter,
		Lim: p.Limit, Off: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list appointments: %w", err)
	}
	total, err := s.q.CountAppointments(ctx, store.CountAppointmentsParams{
		OwnerID: p.Scope.OwnerID, CenterID: p.Scope.CenterID, DonorFilter: p.DonorFilter,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count appointments: %w", err)
	}
	return rows, total, nil
}

func (s *AppointmentService) Get(ctx context.Context, id int32) (store.GetAppointmentRow, error) {
	row, err := s.q.GetAppointment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get appointment %d: %w", id, err)
	}
	return row, nil
}
