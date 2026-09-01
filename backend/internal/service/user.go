package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"bbank/internal/domain"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// InviteTTL is how long an invitation link stays usable.
//
// Short enough that a link forwarded to the wrong address, or left in an inbox
// that is later compromised, stops being a way in; long enough that somebody on
// leave can still use it. Re-inviting is cheap and expected.
const InviteTTL = 7 * 24 * time.Hour

var (
	// ErrLastAdmin guards the one operation that can lock everybody out.
	ErrLastAdmin = errors.New("cannot remove the last active admin")
	// ErrInviteInvalid covers unknown, expired and already-accepted tokens.
	// They are one error on purpose: telling a caller which one it was turns
	// this endpoint into an oracle for guessing valid tokens.
	ErrInviteInvalid = errors.New("this invitation is not valid")
)

// UserService implements FR-66: invite, assign roles, suspend, reactivate.
//
// It replaces the hardcoded `admin@admin.com / admin` branch that used to sit
// at the top of the login action (TD-02). That literal stopped *working* at
// WI-17, when a session became something only the API can sign — this is what
// makes it unnecessary, by giving an operator a supported way to create the
// accounts they previously could not.
type UserService struct {
	pool *pgxpool.Pool
	q    *store.Queries
	auth *AuthService
}

func NewUserService(pool *pgxpool.Pool, q *store.Queries, auth *AuthService) *UserService {
	return &UserService{pool: pool, q: q, auth: auth}
}

type ListUserParams struct {
	Role   *string
	Status *string
	Search *string
	Limit  int32
	Offset int32
}

func (s *UserService) List(ctx context.Context, p ListUserParams) ([]store.ListUsersRow, int64, error) {
	role, err := parseRole(p.Role)
	if err != nil {
		return nil, 0, err
	}
	status, err := parseStatus(p.Status)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.q.ListUsers(ctx, store.ListUsersParams{
		Role: role, Status: status, Search: p.Search, Lim: p.Limit, Off: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	total, err := s.q.CountUsers(ctx, store.CountUsersParams{Role: role, Status: status, Search: p.Search})
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}
	return rows, total, nil
}

func (s *UserService) Get(ctx context.Context, id int64) (store.GetUserRow, error) {
	row, err := s.q.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get user %d: %w", id, err)
	}
	return row, nil
}

type InviteParams struct {
	Email      string
	Role       string
	CenterID   *int64
	HospitalID *int64
	InvitedBy  *int64
}

// Invite creates the account and a single-use token.
//
// The token is returned ONCE and never stored in recoverable form — only its
// SHA-256 lands in the database, exactly as refresh tokens do (WI-17). Delivery
// is the caller's problem for now: `WI-79` sends the email, and until then an
// admin copies the link out of the response. That is a deliberate stopping
// point, not an oversight — inventing a mail path here would be a second
// unaudited way to send credentials.
func (s *UserService) Invite(ctx context.Context, p InviteParams) (userID int64, token string, err error) {
	email := strings.ToLower(strings.TrimSpace(p.Email))
	if email == "" {
		return 0, "", fmt.Errorf("%w: an email address is required", ErrInvalid)
	}
	role, err := domain.ParseRole(p.Role)
	if err != nil {
		return 0, "", fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if err := domain.ValidateRoleScope(role, p.CenterID, p.HospitalID); err != nil {
		return 0, "", fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}

	// A password nobody knows, not a placeholder. The column CHECK requires
	// bcrypt shape, and a shared sentinel would be a backdoor into every
	// invited account that never completes.
	unusable, err := randomSecret()
	if err != nil {
		return 0, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(unusable), bcrypt.DefaultCost)
	if err != nil {
		return 0, "", fmt.Errorf("hash placeholder: %w", err)
	}

	token, err = randomSecret()
	if err != nil {
		return 0, "", err
	}
	sum := sha256.Sum256([]byte(token))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed
	q := s.q.WithTx(tx)

	user, err := q.CreateInvitedUser(ctx, store.CreateInvitedUserParams{
		Email: email, PasswordHash: string(hash), Role: store.UserRole(role),
		CenterID: p.CenterID, HospitalID: p.HospitalID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return 0, "", fmt.Errorf("%w: that email already has an account", ErrConflict)
		}
		if isCheckViolation(err) {
			// Almost always users_center_matches_role — an assignment the schema
			// forbids, e.g. staff with no centre.
			return 0, "", fmt.Errorf("%w: that role and centre combination is not allowed", ErrInvalid)
		}
		return 0, "", fmt.Errorf("create invited user: %w", err)
	}

	if _, err := q.CreateInvite(ctx, store.CreateInviteParams{
		UserID: user.ID, TokenHash: sum[:], InvitedBy: p.InvitedBy,
		ExpiresAt: pgTime(time.Now().Add(InviteTTL)),
	}); err != nil {
		return 0, "", fmt.Errorf("create invite: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, "", fmt.Errorf("commit: %w", err)
	}
	return user.ID, token, nil
}

// AcceptInvite exchanges a token for a working account.
//
// Public by necessity — the invitee has no session yet, which is the entire
// point. The token is the credential, so it is compared by hash and every
// failure mode returns the same error.
func (s *UserService) AcceptInvite(ctx context.Context, token, password string) error {
	if len(password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrInvalid)
	}
	sum := sha256.Sum256([]byte(token))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed
	q := s.q.WithTx(tx)

	inv, err := q.GetOpenInviteByTokenHash(ctx, sum[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInviteInvalid
	}
	if err != nil {
		return fmt.Errorf("lookup invite: %w", err)
	}
	if inv.ExpiresAt.Time.Before(time.Now()) {
		return ErrInviteInvalid
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := q.ActivateUserWithPassword(ctx, store.ActivateUserWithPasswordParams{
		ID: inv.UserID, PasswordHash: string(hash),
	}); err != nil {
		return fmt.Errorf("activate user: %w", err)
	}
	if err := q.AcceptInvite(ctx, inv.ID); err != nil {
		return fmt.Errorf("mark accepted: %w", err)
	}
	return tx.Commit(ctx)
}

// SetStatus suspends, reactivates or deactivates an account.
//
// **Suspending revokes immediately, not at next login** (FR-66). Two things
// make that true: `VerifyAccessToken` re-reads the user's status on every
// request, and this bumps `token_version` and kills the refresh families, so a
// live access token stops verifying at once instead of lingering for its
// remaining minutes.
func (s *UserService) SetStatus(ctx context.Context, id int64, status string, actorID int64) error {
	st, err := parseStatus(&status)
	if err != nil || st == nil {
		return fmt.Errorf("%w: unknown status", ErrInvalid)
	}
	if id == actorID && *st != store.UserStatusActive {
		// Locking yourself out is never the intent, and recovering needs another
		// admin. Refusing is kinder than obeying.
		return fmt.Errorf("%w: you cannot suspend your own account", ErrConflict)
	}

	current, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.Role == store.UserRoleAdmin && *st != store.UserStatusActive {
		if err := s.ensureNotLastAdmin(ctx); err != nil {
			return err
		}
	}

	if err := s.q.SetUserStatus(ctx, store.SetUserStatusParams{ID: id, Status: *st}); err != nil {
		return fmt.Errorf("set status: %w", err)
	}

	if *st != store.UserStatusActive {
		// Sessions go regardless of whether the status re-read would already
		// catch them: leaving a valid refresh family behind for a suspended
		// account is the WI-19 logout defect in another costume.
		if err := s.auth.RevokeAllForUser(ctx, id, "status_"+string(*st)); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
	}
	return nil
}

// SetRole changes a role, and the scope that goes with it.
//
// The session is revoked afterwards because the role is baked into the signed
// token: without a bump, a demoted admin keeps admin authority until their
// access token expires. FR-66 requires the change to take effect on the next
// request.
func (s *UserService) SetRole(ctx context.Context, id int64, role string, centerID, hospitalID *int64, actorID int64) error {
	r, err := domain.ParseRole(role)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if err := domain.ValidateRoleScope(r, centerID, hospitalID); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}

	current, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if id == actorID && r != domain.RoleAdmin {
		return fmt.Errorf("%w: you cannot remove your own admin role", ErrConflict)
	}
	if current.Role == store.UserRoleAdmin && r != domain.RoleAdmin {
		if err := s.ensureNotLastAdmin(ctx); err != nil {
			return err
		}
	}

	if err := s.q.SetUserRole(ctx, store.SetUserRoleParams{
		ID: id, Role: store.UserRole(r), CenterID: centerID, HospitalID: hospitalID,
	}); err != nil {
		if isCheckViolation(err) {
			return fmt.Errorf("%w: that role and centre combination is not allowed", ErrInvalid)
		}
		return fmt.Errorf("set role: %w", err)
	}
	return s.auth.RevokeAllForUser(ctx, id, "role_change")
}

// ensureNotLastAdmin refuses the operation that would leave nobody able to
// perform it. There is no recovery path from zero admins short of SQL.
func (s *UserService) ensureNotLastAdmin(ctx context.Context) error {
	n, err := s.q.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if n <= 1 {
		return fmt.Errorf("%w: %s", ErrConflict, ErrLastAdmin.Error())
	}
	return nil
}

// HasActiveAdmin reports whether anyone can administer this deployment. Used by
// the bootstrap path, which must run only when the answer is no.
func (s *UserService) HasActiveAdmin(ctx context.Context) (bool, error) {
	n, err := s.q.CountAdmins(ctx)
	return n > 0, err
}

// pgTime is the pgtype wrapper the generated params expect.
func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func parseRole(s *string) (*store.UserRole, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	r, err := domain.ParseRole(*s)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	v := store.UserRole(r)
	return &v, nil
}

func parseStatus(s *string) (*store.UserStatus, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	switch v := store.UserStatus(strings.ToLower(strings.TrimSpace(*s))); v {
	case store.UserStatusPendingVerification, store.UserStatusActive,
		store.UserStatusSuspended, store.UserStatusDeactivated:
		return &v, nil
	default:
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalid, *s)
	}
}
