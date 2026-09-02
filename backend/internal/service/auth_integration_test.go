package service_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"bbank/internal/platform"
	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const testPassword = "correct-horse-battery-staple"

func authSetup(t *testing.T) (*service.AuthService, *pgxpool.Pool, int64) {
	t.Helper()
	pool := testsupport.Pool(t)

	// An ephemeral key is right here: these tests are about session lifecycle,
	// not about key management, and generating one keeps a private key out of
	// the test fixtures entirely.
	signer, err := platform.NewSigner("", "https://api.test", "bbank-web", true)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	svc := service.NewAuthService(store.New(pool), signer)

	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	var userID int64
	err = pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, role, status)
		VALUES ('auth@example.test', $1, 'donor', 'active') RETURNING id`, string(hash)).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return svc, pool, userID
}

func login(t *testing.T, svc *service.AuthService) *service.TokenPair {
	t.Helper()
	pair, err := svc.Login(context.Background(), service.LoginInput{
		Email: "auth@example.test", Password: testPassword, IP: "127.0.0.1", UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return pair
}

func TestLoginRejectsAWrongPasswordWithoutSayingWhy(t *testing.T) {
	svc, _, _ := authSetup(t)
	ctx := context.Background()

	_, wrongPass := svc.Login(ctx, service.LoginInput{Email: "auth@example.test", Password: "nope"})
	_, noSuchUser := svc.Login(ctx, service.LoginInput{Email: "ghost@example.test", Password: testPassword})

	if wrongPass == nil || noSuchUser == nil {
		t.Fatal("a bad credential was accepted")
	}
	// The two must be indistinguishable, or login becomes a way to enumerate
	// who has an account here (NFR-12).
	if wrongPass.Error() != noSuchUser.Error() {
		t.Errorf("wrong password says %q but unknown account says %q — that difference enumerates users",
			wrongPass, noSuchUser)
	}
}

func TestLoginIssuesAVerifiableTokenAndASession(t *testing.T) {
	svc, pool, userID := authSetup(t)
	pair := login(t, svc)

	claims, err := svc.VerifyAccessToken(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatalf("the freshly issued token does not verify: %v", err)
	}
	// The user id lives in the standard `sub` claim, not a bespoke field.
	if claims.Subject != strconv.FormatInt(userID, 10) {
		t.Errorf("sub = %q, want %d", claims.Subject, userID)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, userID); n != 1 {
		t.Errorf("got %d live sessions after login, want 1", n)
	}

	// Only the SHA-256 of the refresh token is stored: a database disclosure
	// must not hand the attacker usable sessions.
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM sessions WHERE encode(token_hash,'hex') = $1`, pair.RefreshToken); n != 0 {
		t.Error("the refresh token itself appears in the sessions table")
	}
}

func TestRefreshRotatesTheToken(t *testing.T) {
	svc, _, _ := authSetup(t)
	first := login(t, svc)

	second, err := svc.Refresh(context.Background(), first.RefreshToken, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh returned the same token; rotation did not happen")
	}
	if _, err := svc.VerifyAccessToken(context.Background(), second.AccessToken); err != nil {
		t.Errorf("the rotated access token does not verify: %v", err)
	}
}

// The security property WI-17 built and only ever verified by hand.
//
// Presenting an already-rotated refresh token means one of two things: a
// legitimate client replayed, or a stolen token is being used. There is no way
// to tell them apart, so the whole family is revoked — the thief and the victim
// are both signed out, which is the safe direction to fail.
func TestReplayingARotatedRefreshTokenRevokesTheWholeFamily(t *testing.T) {
	svc, pool, userID := authSetup(t)
	ctx := context.Background()

	stolen := login(t, svc)
	legitimate, err := svc.Refresh(ctx, stolen.RefreshToken, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	// The attacker replays the token they captured before rotation.
	if _, err := svc.Refresh(ctx, stolen.RefreshToken, "10.0.0.1", "attacker"); !errors.Is(err, service.ErrRefreshReused) {
		t.Fatalf("replaying a rotated token = %v, want ErrRefreshReused", err)
	}

	// And the legitimate token — obtained honestly — is dead too.
	if _, err := svc.Refresh(ctx, legitimate.RefreshToken, "127.0.0.1", "test"); err == nil {
		t.Fatal("the legitimate token still works after reuse was detected; the family was not revoked")
	}

	live := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if live != 0 {
		t.Errorf("%d sessions survived family revocation, want 0", live)
	}
}

func TestLogoutRevokesTheFamily(t *testing.T) {
	svc, pool, userID := authSetup(t)
	ctx := context.Background()

	pair := login(t, svc)
	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := svc.Refresh(ctx, pair.RefreshToken, "127.0.0.1", "test"); err == nil {
		t.Fatal("the refresh token still works after logout")
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NOT NULL AND revoked_reason = 'logout'`,
		userID); n == 0 {
		t.Error("logout did not record a revocation reason")
	}
}

// token_version is the escape hatch from the 15-minute access-token window: a
// role change, password change or forced logout bumps it, and every outstanding
// access token stops verifying immediately rather than at expiry.
func TestBumpingTokenVersionInvalidatesLiveAccessTokens(t *testing.T) {
	svc, pool, userID := authSetup(t)
	ctx := context.Background()

	pair := login(t, svc)
	if _, err := svc.VerifyAccessToken(ctx, pair.AccessToken); err != nil {
		t.Fatalf("token does not verify before the bump: %v", err)
	}

	if err := svc.RevokeAllForUser(ctx, userID, "role_change"); err != nil {
		t.Fatalf("revoke all: %v", err)
	}

	if _, err := svc.VerifyAccessToken(ctx, pair.AccessToken); err == nil {
		t.Fatal("an access token still verifies after token_version was bumped")
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, userID); n != 0 {
		t.Error("sessions survived a forced logout")
	}
}

func TestGarbageRefreshTokenIsRejected(t *testing.T) {
	svc, _, _ := authSetup(t)
	ctx := context.Background()

	for _, tok := range []string{"", "not-a-token", "0000000000000000000000000000000000000000"} {
		if _, err := svc.Refresh(ctx, tok, "127.0.0.1", "test"); !errors.Is(err, service.ErrRefreshInvalid) {
			t.Errorf("Refresh(%q) = %v, want ErrRefreshInvalid", tok, err)
		}
	}
}

// A suspended account must not be able to log in, and the check has to be on
// the account rather than only on the credential.
func TestInactiveAccountCannotLogIn(t *testing.T) {
	svc, pool, userID := authSetup(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := svc.Login(ctx, service.LoginInput{Email: "auth@example.test", Password: testPassword}); err == nil {
		t.Fatal("a suspended account logged in")
	}
}

// NFR-12: repeated failures must actually lock the account.
//
// `RecordFailedLogin` incremented the counter and never set `locked_until`, so
// the ErrAccountLocked branch, the 423 status and the frontend's "temporarily
// locked" message were all unreachable — online password guessing was unbounded.
func TestRepeatedFailuresLockTheAccount(t *testing.T) {
	svc, pool, userID := authSetup(t)
	ctx := context.Background()

	for i := 0; i < service.MaxFailedLogins; i++ {
		if _, err := svc.Login(ctx, service.LoginInput{Email: "auth@example.test", Password: "wrong"}); err == nil {
			t.Fatal("a wrong password was accepted")
		}
	}

	var locked bool
	if err := pool.QueryRow(ctx,
		`SELECT locked_until IS NOT NULL AND locked_until > now() FROM users WHERE id = $1`, userID).Scan(&locked); err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if !locked {
		t.Fatalf("after %d failures the account is not locked; guessing is unbounded", service.MaxFailedLogins)
	}

	// The CORRECT password is now refused too — that is the point of a lockout.
	if _, err := svc.Login(ctx, service.LoginInput{Email: "auth@example.test", Password: testPassword}); !errors.Is(err, service.ErrAccountLocked) {
		t.Fatalf("login while locked = %v, want ErrAccountLocked", err)
	}

	// A successful login after the window clears the counter and the lock, so a
	// user who mistyped is not penalised forever.
	if _, err := pool.Exec(ctx, `UPDATE users SET locked_until = now() - INTERVAL '1 minute' WHERE id = $1`, userID); err != nil {
		t.Fatalf("expire the lock: %v", err)
	}
	if _, err := svc.Login(ctx, service.LoginInput{Email: "auth@example.test", Password: testPassword}); err != nil {
		t.Fatalf("login after the lock expired: %v", err)
	}
	var count int32
	if err := pool.QueryRow(ctx, `SELECT failed_login_count FROM users WHERE id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if count != 0 {
		t.Errorf("failed_login_count = %d after a successful login, want 0", count)
	}
}

// A lockout must not become an enumeration oracle: an unknown address cannot be
// locked, so it must not answer differently from a locked real one.
func TestLockoutDoesNotRevealWhichAccountsExist(t *testing.T) {
	svc, _, _ := authSetup(t)
	ctx := context.Background()

	for i := 0; i < service.MaxFailedLogins+2; i++ {
		if _, err := svc.Login(ctx, service.LoginInput{Email: "ghost@example.test", Password: "wrong"}); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("an unknown account answered %v; it must stay indistinguishable", err)
		}
	}
}
