// Package http_test drives the REAL router against a REAL database (WI-30).
//
// Everything below WI-29's service tests was already covered. What was not, and
// what three consecutive work items verified only by hand, is the HTTP layer
// itself: the middleware chain in the order it actually runs, the routes as
// they are actually mounted, and the status codes a client actually sees.
//
// The distinction matters. A service test proves the rule is implemented; only
// an HTTP test proves the rule is *mounted*. WI-20's changelog says as much —
// "a matrix that passes its unit tests can still be mounted on the wrong
// routes" — and answered it by hand, once, against a running stack. These are
// the same checks, run by CI, every time.
//
// Each test is named for the requirement or defect it guards, per WI-30's
// acceptance criterion: a failure should say what broke, not merely that
// something did.
package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bbhttp "bbank/internal/http"
	"bbank/internal/platform"
	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const password = "harness-password-123"

type harness struct {
	srv    http.Handler
	pool   *pgxpool.Pool
	auth   *service.AuthService
	flags  *platform.Flags
	signer *platform.Signer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pool := testsupport.Pool(t)

	signer, err := platform.NewSigner("", "https://api.test", "bbank-web", true)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	flags := platform.NewFlags(true)

	cfg := platform.Config{
		JWTIssuer:   "https://api.test",
		JWTAudience: "bbank-web",
		LegacyShim:  true,
	}
	// The real router, with the real middleware chain in the real order.
	srv := bbhttp.NewRouter(bbhttp.Deps{Cfg: cfg, Pool: pool, Signer: signer, Flags: flags})

	return &harness{
		srv: srv, pool: pool, flags: flags, signer: signer,
		auth: service.NewAuthService(store.New(pool), signer),
	}
}

// user inserts an account with a known password and returns its id.
func (h *harness) user(t *testing.T, email, role string, centerID *int64) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	var id int64
	err = h.pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, role, status, center_id)
		VALUES ($1, $2, $3::user_role, 'active', $4) RETURNING id`,
		email, string(hash), role, centerID).Scan(&id)
	if err != nil {
		t.Fatalf("insert %s: %v", role, err)
	}
	return id
}

// donor inserts a user + donor_profiles pair, which donation requests need.
func (h *harness) donor(t *testing.T, email, name string) int64 {
	t.Helper()
	id := h.user(t, email, "donor", nil)
	if _, err := h.pool.Exec(context.Background(), `
		INSERT INTO donor_profiles (user_id, full_name, date_of_birth, gender, contact_phone)
		VALUES ($1, $2, DATE '1990-01-01', 'undisclosed', '+237600000000')`, id, name); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	return id
}

// token logs the account in through the service, so the token under test is one
// the system actually issues rather than one the test forged.
func (h *harness) token(t *testing.T, email string) string {
	t.Helper()
	pair, err := h.auth.Login(context.Background(), service.LoginInput{Email: email, Password: password})
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return pair.AccessToken
}

type reply struct {
	status int
	body   string
	header http.Header
}

// do sends a request through the real router. An empty token means anonymous.
func (h *harness) do(t *testing.T, method, path, token, body string) reply {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "bb_at", Value: token})
	}

	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, r)

	res := rec.Result()
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return reply{status: res.StatusCode, body: string(raw), header: res.Header}
}

// errorCode pulls the machine-readable code clients are supposed to branch on.
func (r reply) errorCode(t *testing.T) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(r.body), &env); err != nil {
		return ""
	}
	return env.Error.Code
}

func (r reply) page(t *testing.T) map[string]any {
	t.Helper()
	var env struct {
		Page map[string]any `json:"page"`
	}
	if err := json.Unmarshal([]byte(r.body), &env); err != nil {
		t.Fatalf("response is not an envelope: %s", r.body)
	}
	return env.Page
}
