package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"bbank/internal/platform"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountLocked      = errors.New("account is locked")
	ErrAccountInactive    = errors.New("account is not active")
	ErrRefreshInvalid     = errors.New("refresh token is invalid or expired")
	ErrRefreshReused      = errors.New("refresh token was already used")
)

// bcrypt cost 12, up from the library default of 10 (TRD §5.1). ~250ms on
// commodity hardware, which is acceptable for a rate-limited login endpoint and
// materially harder to brute force offline.
const BcryptCost = 12

type AuthService struct {
	q      *store.Queries
	signer *platform.Signer
}

func NewAuthService(q *store.Queries, s *platform.Signer) *AuthService {
	return &AuthService{q: q, signer: s}
}

type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	SessionID        string
	UserID           int64
	Role             string
}

type LoginInput struct {
	Email     string
	Password  string
	IP        string
	UserAgent string
}

func hashToken(t string) []byte {
	sum := sha256.Sum256([]byte(t))
	return sum[:]
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (*TokenPair, error) {
	u, err := s.q.GetUserForLogin(ctx, in.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		// Spend the time anyway: an early return here turns login into a user
		// enumeration oracle, because "no such account" would be measurably
		// faster than "wrong password".
		_, _ = bcrypt.GenerateFromPassword([]byte(in.Password), BcryptCost)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if u.LockedUntil.Valid && u.LockedUntil.Time.After(time.Now()) {
		return nil, ErrAccountLocked
	}
	if u.Status != store.UserStatusActive {
		return nil, ErrAccountInactive
	}

	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		if err := s.q.RecordFailedLogin(ctx, in.Email); err != nil {
			slog.ErrorContext(ctx, "cannot record failed login", "error", err)
		}
		return nil, ErrInvalidCredentials
	}

	if err := s.q.TouchLastLogin(ctx, u.ID); err != nil {
		slog.ErrorContext(ctx, "cannot touch last_login_at", "error", err)
	}

	return s.issuePair(ctx, issueInput{
		userID:       u.ID,
		role:         string(u.Role),
		centerID:     u.CenterID,
		hospitalID:   u.HospitalID,
		tokenVersion: u.TokenVersion,
		familyID:     platform.NewID("fam"),
		ip:           in.IP,
		userAgent:    in.UserAgent,
	})
}

type issueInput struct {
	userID int64
	role   string
	// centerID and hospitalID become the `cid` and `hid` claims. They are the
	// only source of ctr/hosp scoping in the RBAC middleware (TRD §7.6/§7.7),
	// so a nil here is not cosmetic: it denies that role every scoped row.
	centerID     *int64
	hospitalID   *int64
	tokenVersion int32
	familyID     string
	ip           string
	userAgent    string
}

func (s *AuthService) issuePair(ctx context.Context, in issueInput) (*TokenPair, error) {
	sessionID := platform.NewID("ses")
	refresh := platform.NewOpaqueToken()
	refreshExp := time.Now().Add(platform.RefreshTokenTTL)

	access, accessExp, err := s.signer.SignAccessToken(platform.TokenSubject{
		UserID:       in.userID,
		SessionID:    sessionID,
		Role:         in.role,
		CenterID:     in.centerID,
		HospitalID:   in.hospitalID,
		TokenVersion: in.tokenVersion,
	}, time.Now())
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	if _, err := s.q.CreateSession(ctx, store.CreateSessionParams{
		PublicID:  sessionID,
		UserID:    in.userID,
		FamilyID:  in.familyID,
		TokenHash: hashToken(refresh),
		ExpiresAt: pgtype.Timestamptz{Time: refreshExp, Valid: true},
		Ip:        parseIP(in.ip),
		UserAgent: strPtrOrNil(in.userAgent),
	}); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &TokenPair{
		AccessToken: access, AccessExpiresAt: accessExp,
		RefreshToken: refresh, RefreshExpiresAt: refreshExp,
		SessionID: sessionID, UserID: in.userID, Role: in.role,
	}, nil
}

// Refresh rotates the pair. Presenting an already-rotated or revoked token
// revokes the ENTIRE family: that pattern means the token was stolen and both
// the attacker and the legitimate user are now holding one.
func (s *AuthService) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*TokenPair, error) {
	sess, err := s.q.GetSessionByTokenHash(ctx, hashToken(refreshToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRefreshInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("lookup session: %w", err)
	}

	// Constant-time compare on the hash as well. The unique index already found
	// it, but this costs nothing and removes a timing signal.
	if subtle.ConstantTimeCompare(sess.TokenHash, hashToken(refreshToken)) != 1 {
		return nil, ErrRefreshInvalid
	}

	if sess.RevokedAt.Valid {
		return nil, ErrRefreshInvalid
	}
	if sess.RotatedAt.Valid {
		// REUSE DETECTED.
		n, rerr := s.q.RevokeSessionFamily(ctx, store.RevokeSessionFamilyParams{
			FamilyID: sess.FamilyID, Reason: strPtr("refresh_reuse_detected"),
		})
		if rerr != nil {
			slog.ErrorContext(ctx, "cannot revoke session family after reuse", "error", rerr)
		}
		slog.WarnContext(ctx, "security.refresh_reuse",
			"user_id", sess.UserID, "family_id", sess.FamilyID, "revoked_sessions", n)
		return nil, ErrRefreshReused
	}
	if sess.ExpiresAt.Valid && sess.ExpiresAt.Time.Before(time.Now()) {
		return nil, ErrRefreshInvalid
	}

	u, err := s.q.GetUserForToken(ctx, sess.UserID)
	if err != nil {
		return nil, fmt.Errorf("lookup user for refresh: %w", err)
	}
	if u.Status != store.UserStatusActive {
		return nil, ErrAccountInactive
	}

	if err := s.q.MarkSessionRotated(ctx, sess.ID); err != nil {
		return nil, fmt.Errorf("mark rotated: %w", err)
	}

	return s.issuePair(ctx, issueInput{
		userID: u.ID, role: string(u.Role), centerID: u.CenterID, hospitalID: u.HospitalID,
		tokenVersion: u.TokenVersion, familyID: sess.FamilyID,
		ip: ip, userAgent: userAgent,
	})
}

// Logout revokes the presented token's whole family.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	sess, err := s.q.GetSessionByTokenHash(ctx, hashToken(refreshToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already gone; logging out twice is not an error
	}
	if err != nil {
		return fmt.Errorf("lookup session: %w", err)
	}
	_, err = s.q.RevokeSessionFamily(ctx, store.RevokeSessionFamilyParams{
		FamilyID: sess.FamilyID, Reason: strPtr("logout"),
	})
	return err
}

// VerifyAccessToken checks the signature AND that the token_version still
// matches. The version check needs a database read, which is why it lives here
// and not in platform.Signer.
func (s *AuthService) VerifyAccessToken(ctx context.Context, token string) (*platform.Claims, error) {
	claims, err := s.signer.Verify(token)
	if err != nil {
		return nil, err
	}
	var userID int64
	if _, err := fmt.Sscanf(claims.Subject, "%d", &userID); err != nil {
		return nil, platform.ErrTokenInvalid
	}
	u, err := s.q.GetUserForToken(ctx, userID)
	if err != nil {
		return nil, platform.ErrTokenInvalid
	}
	if u.TokenVersion != claims.TokenVersion {
		return nil, platform.ErrTokenVerMismatch
	}
	if u.Status != store.UserStatusActive {
		return nil, ErrAccountInactive
	}
	return claims, nil
}

// RevokeAllForUser is the forced-logout path: bump the version (killing every
// outstanding access token now) and revoke every refresh family.
func (s *AuthService) RevokeAllForUser(ctx context.Context, userID int64, reason string) error {
	if err := s.q.BumpTokenVersion(ctx, userID); err != nil {
		return fmt.Errorf("bump token version: %w", err)
	}
	_, err := s.q.RevokeSessionsForUser(ctx, store.RevokeSessionsForUserParams{
		UserID: userID, Reason: strPtr(reason),
	})
	return err
}

func strPtr(s string) *string { return &s }

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseIP(s string) *netip.Addr {
	if s == "" {
		return nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return nil
	}
	return &a
}
