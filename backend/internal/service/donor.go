// Package service holds use cases: transaction boundaries, authorization
// decisions, and orchestration between store and domain.
//
// Dependency rule: service may import domain and store. It must not import http.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"bbank/internal/domain"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
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

// CreateParams is a donor registration: one `users` row and one
// `donor_profiles` row, written together.
type CreateParams struct {
	Email       string
	Password    string
	FullName    string
	DateOfBirth time.Time
	Gender      string
	BloodGroup  *string
	Rhesus      *string
	Phone       string
	Address     *string
}

// Create registers a donor.
//
// This endpoint has not existed since WI-11 moved donors to the layered
// handlers with reads only, which is why signup has been switched off. The
// query it uses (`CreateDonor`) writes `users` and `donor_profiles` in a single
// statement with a CTE, so there is no window in which a user exists without a
// profile — the state that made `createRequest` fail on a foreign key and
// return 500 for what was really a missing profile.
//
// **Blood group is deliberately not accepted from a self-registering donor.**
// It is a laboratory result (FR-21), not a self-reported attribute, and the
// original system let people type it in — which is how "O+", "o" and " A "
// ended up in one column (defect D7). Staff may set it; the value a donor types
// about their own blood is not evidence.
func (s *DonorService) Create(ctx context.Context, p CreateParams, allowClinical bool) (int64, error) {
	email := strings.ToLower(strings.TrimSpace(p.Email))

	d := domain.Donor{Email: email, FullName: strings.TrimSpace(p.FullName), Phone: p.Phone}
	if allowClinical && p.BloodGroup != nil && p.Rhesus != nil {
		d.BloodGroup = domain.BloodGroup(*p.BloodGroup)
		d.Rhesus = domain.Rhesus(*p.Rhesus)
	}
	if err := d.Validate(); err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if len(p.Password) < 8 {
		return 0, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalid)
	}

	gender, err := domain.ParseGender(p.Gender)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(p.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash password: %w", err)
	}

	arg := store.CreateDonorParams{
		Email:        email,
		PasswordHash: string(hash),
		FullName:     d.FullName,
		DateOfBirth:  pgtype.Date{Time: p.DateOfBirth, Valid: !p.DateOfBirth.IsZero()},
		Gender:       store.Gender(gender),
		ContactPhone: p.Phone,
		AddressLine:  p.Address,
	}
	if d.BloodGroup != "" {
		bg, rh := store.BloodGroup(d.BloodGroup), store.Rhesus(d.Rhesus)
		arg.BloodGroup, arg.Rhesus = &bg, &rh
	}

	id, err := s.q.CreateDonor(ctx, arg)
	if err != nil {
		// Uniqueness on users.email is the only thing that actually holds under
		// concurrent signups; checking first and trusting the answer is a race.
		if isUniqueViolation(err) {
			return 0, fmt.Errorf("%w: that email is already registered", ErrConflict)
		}
		if isCheckViolation(err) {
			return 0, fmt.Errorf("%w: those details are not a valid donor record", ErrInvalid)
		}
		return 0, fmt.Errorf("create donor: %w", err)
	}
	return id, nil
}

// UpdateParams is a profile edit. Clinical fields are separated from the ones a
// donor may set about themselves, and the handler decides which set applies.
type UpdateParams struct {
	FullName              string
	DateOfBirth           time.Time
	Gender                string
	BloodGroup            *string
	Rhesus                *string
	Phone                 string
	Address               *string
	City                  *string
	Region                *string
	NationalID            *string
	EmergencyContactName  *string
	EmergencyContactPhone *string
}

// Update rewrites a donor profile.
//
// `allowClinical` gates blood group and rhesus for the same reason as Create:
// they are lab results. A donor editing their own profile passes false, and the
// existing values are carried through untouched rather than being blanked —
// silently erasing a typed blood group because the edit form did not send it
// would be worse than refusing the edit.
func (s *DonorService) Update(ctx context.Context, id int64, p UpdateParams, allowClinical bool) error {
	current, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	gender, err := domain.ParseGender(p.Gender)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}

	d := domain.Donor{Email: current.Email, FullName: strings.TrimSpace(p.FullName), Phone: p.Phone}
	arg := store.UpdateDonorProfileParams{
		UserID:                id,
		FullName:              d.FullName,
		DateOfBirth:           pgtype.Date{Time: p.DateOfBirth, Valid: !p.DateOfBirth.IsZero()},
		Gender:                store.Gender(gender),
		ContactPhone:          p.Phone,
		AddressLine:           p.Address,
		City:                  p.City,
		Region:                p.Region,
		NationalID:            p.NationalID,
		EmergencyContactName:  p.EmergencyContactName,
		EmergencyContactPhone: p.EmergencyContactPhone,
	}

	// Carry the stored clinical values forward unless a caller entitled to
	// change them supplied new ones.
	arg.BloodGroup, arg.Rhesus = current.BloodGroup, current.Rhesus
	if allowClinical && p.BloodGroup != nil && p.Rhesus != nil {
		d.BloodGroup, d.Rhesus = domain.BloodGroup(*p.BloodGroup), domain.Rhesus(*p.Rhesus)
		bg, rh := store.BloodGroup(*p.BloodGroup), store.Rhesus(*p.Rhesus)
		arg.BloodGroup, arg.Rhesus = &bg, &rh
	}
	if err := d.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}

	if err := s.q.UpdateDonorProfile(ctx, arg); err != nil {
		if isCheckViolation(err) {
			return fmt.Errorf("%w: those details are not a valid donor record", ErrInvalid)
		}
		return fmt.Errorf("update donor %d: %w", id, err)
	}
	return nil
}
