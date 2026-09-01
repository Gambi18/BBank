// Package service holds use cases: transaction boundaries, authorization
// decisions, and orchestration between store and domain.
//
// Dependency rule: service may import domain and store. It must not import http.
package service

import (
	"context"
	"errors"
	"fmt"

	"bbank/internal/domain"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

type DonorService struct {
	q *store.Queries
}

func NewDonorService(q *store.Queries) *DonorService { return &DonorService{q: q} }

// ListParams carries pagination. Defaults are applied here rather than in the
// handler so every caller gets a bounded query — the legacy /donors endpoint
// scanned the whole table (TD-17).
type ListParams struct {
	Search *string
	Limit  int32
	Offset int32
}

const (
	defaultLimit = 25
	maxLimit     = 100
)

func (p *ListParams) normalise() {
	if p.Limit <= 0 {
		p.Limit = defaultLimit
	}
	if p.Limit > maxLimit {
		p.Limit = maxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
}

func (s *DonorService) List(ctx context.Context, p ListParams) ([]store.ListDonorsRow, int64, error) {
	p.normalise()
	rows, err := s.q.ListDonors(ctx, store.ListDonorsParams{Search: p.Search, Lim: p.Limit, Off: p.Offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list donors: %w", err)
	}
	total, err := s.q.CountDonors(ctx, p.Search)
	if err != nil {
		return nil, 0, fmt.Errorf("count donors: %w", err)
	}
	return rows, total, nil
}

func (s *DonorService) Get(ctx context.Context, id int64) (store.GetDonorByIDRow, error) {
	row, err := s.q.GetDonorByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get donor %d: %w", id, err)
	}
	return row, nil
}

// Eligibility reads the donor_eligibility view. Deliberately a read of computed
// state: there is no setter, because eligibility is derived from real donation
// records plus active deferrals plus policy — never from a stored field.
func (s *DonorService) Eligibility(ctx context.Context, id int64) (store.DonorEligibility, error) {
	row, err := s.q.GetDonorEligibility(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get eligibility %d: %w", id, err)
	}
	return row, nil
}

// ValidateDonor runs the domain invariants. Kept on the service so handlers stay
// thin and the rule is enforced regardless of which transport calls it.
func (s *DonorService) ValidateDonor(d domain.Donor) error { return d.Validate() }
