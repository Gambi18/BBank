package service_test

import (
	"context"
	"errors"
	"testing"

	"bbank/internal/platform"
	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
)

func userSetup(t *testing.T) (*service.UserService, *service.AuthService, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.Pool(t)
	signer, err := platform.NewSigner("", "https://api.test", "bbank-web", true)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	q := store.New(pool)
	auth := service.NewAuthService(q, signer)
	return service.NewUserService(pool, q, auth), auth, pool
}

// The whole point of WI-18: an operator can create the accounts the hardcoded
// credential used to stand in for, and the new account works.
func TestInviteThenAcceptProducesAWorkingAccount(t *testing.T) {
	users, auth, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	id, token, err := users.Invite(ctx, service.InviteParams{
		Email: "newstaff@example.test", Role: "staff", CenterID: &center,
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}

	// Before acceptance the account exists but cannot be used: no password
	// anyone knows, and a status that login refuses.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM users WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "pending_verification" {
		t.Errorf("invited user status = %q, want pending_verification", status)
	}
	if _, err := auth.Login(ctx, service.LoginInput{Email: "newstaff@example.test", Password: "anything"}); err == nil {
		t.Fatal("an un-accepted invitation could log in")
	}

	if err := users.AcceptInvite(ctx, token, "a-real-password"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := auth.Login(ctx, service.LoginInput{Email: "newstaff@example.test", Password: "a-real-password"}); err != nil {
		t.Fatalf("the accepted account cannot log in: %v", err)
	}
}

// The token is a credential: it must be single-use, and every failure mode must
// look the same so the endpoint cannot be used to probe for live tokens.
func TestInviteTokensAreSingleUseAndOpaqueOnFailure(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	_, token, err := users.Invite(ctx, service.InviteParams{
		Email: "once@example.test", Role: "staff", CenterID: &center,
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := users.AcceptInvite(ctx, token, "first-password"); err != nil {
		t.Fatalf("first accept: %v", err)
	}

	reuse := users.AcceptInvite(ctx, token, "second-password")
	unknown := users.AcceptInvite(ctx, "a-token-that-never-existed", "second-password")
	if !errors.Is(reuse, service.ErrInviteInvalid) {
		t.Fatalf("reusing a token = %v, want ErrInviteInvalid", reuse)
	}
	if !errors.Is(unknown, service.ErrInviteInvalid) {
		t.Fatalf("an unknown token = %v, want ErrInviteInvalid", unknown)
	}
	if reuse.Error() != unknown.Error() {
		t.Error("a used token and an unknown token give different answers; that distinguishes live tokens")
	}
}

// Only the hash is stored. A database disclosure must not hand over usable
// invitations, the same rule refresh tokens follow.
func TestInviteTokenIsNotStoredInRecoverableForm(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()

	_, token, err := users.Invite(ctx, service.InviteParams{Email: "hashed@example.test", Role: "admin"})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM user_invites WHERE encode(token_hash,'hex') = $1`, token); n != 0 {
		t.Error("the invite token itself is in the table")
	}
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM user_invites`); n != 1 {
		t.Error("the invite row is missing")
	}
}

// **The FR-66 acceptance criterion.** Suspending must invalidate the session on
// the next REQUEST, not at the next login — otherwise a compromised or departing
// account keeps working for as long as its token lives.
func TestSuspendingInvalidatesALiveSessionImmediately(t *testing.T) {
	users, auth, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	admin := testsupport.NewAdmin(t, pool, "boss@example.test")

	id, token, err := users.Invite(ctx, service.InviteParams{
		Email: "leaver@example.test", Role: "staff", CenterID: &center,
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := users.AcceptInvite(ctx, token, "leaver-password"); err != nil {
		t.Fatalf("accept: %v", err)
	}

	pair, err := auth.Login(ctx, service.LoginInput{Email: "leaver@example.test", Password: "leaver-password"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := auth.VerifyAccessToken(ctx, pair.AccessToken); err != nil {
		t.Fatalf("the token does not verify before suspension: %v", err)
	}

	if err := users.SetStatus(ctx, id, "suspended", admin); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	// The token is still cryptographically valid and unexpired. It must stop
	// working anyway.
	if _, err := auth.VerifyAccessToken(ctx, pair.AccessToken); err == nil {
		t.Fatal("a suspended user's access token still verifies — the session survives until it expires")
	}
	// And the refresh family is gone, so they cannot mint a new one.
	if _, err := auth.Refresh(ctx, pair.RefreshToken, "127.0.0.1", "test"); err == nil {
		t.Fatal("a suspended user could refresh into a new session")
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL`, id); n != 0 {
		t.Errorf("%d sessions survived suspension", n)
	}
}

// Reactivation restores access rather than requiring a new invitation.
func TestReactivationRestoresLogin(t *testing.T) {
	users, auth, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	admin := testsupport.NewAdmin(t, pool, "boss2@example.test")

	id, token, err := users.Invite(ctx, service.InviteParams{
		Email: "back@example.test", Role: "staff", CenterID: &center,
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := users.AcceptInvite(ctx, token, "back-password"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := users.SetStatus(ctx, id, "suspended", admin); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, err := auth.Login(ctx, service.LoginInput{Email: "back@example.test", Password: "back-password"}); err == nil {
		t.Fatal("a suspended user logged in")
	}

	if err := users.SetStatus(ctx, id, "active", admin); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if _, err := auth.Login(ctx, service.LoginInput{Email: "back@example.test", Password: "back-password"}); err != nil {
		t.Fatalf("a reactivated user cannot log in: %v", err)
	}
}

// FR-66: role changes take effect on the next request, not the next login.
func TestRoleChangeTakesEffectOnTheNextRequest(t *testing.T) {
	users, auth, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	admin := testsupport.NewAdmin(t, pool, "boss3@example.test")
	// A second admin, so demoting the first is not blocked by the last-admin rule.
	testsupport.NewAdmin(t, pool, "boss4@example.test")

	id, token, err := users.Invite(ctx, service.InviteParams{
		Email: "promoted@example.test", Role: "staff", CenterID: &center,
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := users.AcceptInvite(ctx, token, "promo-password"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	pair, err := auth.Login(ctx, service.LoginInput{Email: "promoted@example.test", Password: "promo-password"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := users.SetRole(ctx, id, "admin", nil, nil, admin); err != nil {
		t.Fatalf("promote: %v", err)
	}

	// The old token carries `role: staff`. It must stop verifying rather than
	// continue to assert a role the user no longer has.
	if _, err := auth.VerifyAccessToken(ctx, pair.AccessToken); err == nil {
		t.Fatal("a token minted before the role change still verifies")
	}
	var role string
	if err := pool.QueryRow(ctx, `SELECT role::text FROM users WHERE id = $1`, id).Scan(&role); err != nil {
		t.Fatalf("read role: %v", err)
	}
	if role != "admin" {
		t.Errorf("role = %q, want admin", role)
	}
}

// The operation that can lock everybody out of their own system.
func TestTheLastAdminCannotBeRemoved(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()
	only := testsupport.NewAdmin(t, pool, "solo@example.test")
	other := testsupport.NewAdmin(t, pool, "second@example.test")

	// With two admins, demoting one is fine.
	if err := users.SetRole(ctx, only, "lab_tech", nil, nil, other); err != nil {
		t.Fatalf("demoting one of two admins: %v", err)
	}
	// Now `other` is the last one.
	if err := users.SetStatus(ctx, other, "suspended", only); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("suspending the last admin = %v, want ErrConflict", err)
	}
	if err := users.SetRole(ctx, other, "donor", nil, nil, only); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("demoting the last admin = %v, want ErrConflict", err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active'`); n != 1 {
		t.Fatal("the last admin was removed")
	}
}

// Suspending yourself needs another admin to undo. Refusing is kinder.
func TestAnAdminCannotSuspendThemselves(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()
	me := testsupport.NewAdmin(t, pool, "me@example.test")
	testsupport.NewAdmin(t, pool, "colleague@example.test")

	if err := users.SetStatus(ctx, me, "suspended", me); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("self-suspension = %v, want ErrConflict", err)
	}
	if err := users.SetRole(ctx, me, "donor", nil, nil, me); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("self-demotion = %v, want ErrConflict", err)
	}
}

// The role/scope pairing is refused in front of the caller, so an impossible
// assignment is a validation error rather than a constraint violation.
func TestRoleScopeRulesAreEnforced(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	cases := []struct {
		name       string
		role       string
		centerID   *int64
		hospitalID *int64
	}{
		{"staff with no centre", "staff", nil, nil},
		{"admin homed at a centre", "admin", &center, nil},
		{"donor homed at a centre", "donor", &center, nil},
		{"hospital user with no hospital", "hospital_user", nil, nil},
		{"unknown role", "wizard", nil, nil},
	}
	for _, c := range cases {
		_, _, err := users.Invite(ctx, service.InviteParams{
			Email: "scope-" + c.name + "@example.test", Role: c.role,
			CenterID: c.centerID, HospitalID: c.hospitalID,
		})
		if !errors.Is(err, service.ErrInvalid) {
			t.Errorf("%s = %v, want ErrInvalid", c.name, err)
		}
	}
}

func TestInvitingAnExistingEmailIsAConflict(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()
	testsupport.NewDonor(t, pool, "taken@example.test", "Taken Already")

	if _, _, err := users.Invite(ctx, service.InviteParams{Email: "taken@example.test", Role: "admin"}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("inviting an existing email = %v, want ErrConflict", err)
	}
}

// The admin console's list, with the filters it offers.
func TestUserListFiltersAndInvitePendingFlag(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	testsupport.NewAdmin(t, pool, "listadmin@example.test")
	testsupport.NewDonor(t, pool, "listdonor@example.test", "List Donor")
	if _, _, err := users.Invite(ctx, service.InviteParams{
		Email: "listinvited@example.test", Role: "staff", CenterID: &center,
	}); err != nil {
		t.Fatalf("invite: %v", err)
	}

	all, total, err := users.List(ctx, service.ListUserParams{Limit: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("list returned %d / total %d, want 3", len(all), total)
	}

	// An outstanding invitation is visible, so "did I invite them?" is
	// answerable without reading the invites table.
	var pending, accepted int
	for _, u := range all {
		if u.InvitePending {
			pending++
		} else {
			accepted++
		}
	}
	if pending != 1 || accepted != 2 {
		t.Errorf("invite_pending: %d pending / %d not, want 1 / 2", pending, accepted)
	}

	role := "staff"
	_, staffTotal, err := users.List(ctx, service.ListUserParams{Role: &role, Limit: 25})
	if err != nil {
		t.Fatalf("filter by role: %v", err)
	}
	if staffTotal != 1 {
		t.Errorf("role=staff matched %d, want 1", staffTotal)
	}

	status := "pending_verification"
	_, pendingTotal, err := users.List(ctx, service.ListUserParams{Status: &status, Limit: 25})
	if err != nil {
		t.Fatalf("filter by status: %v", err)
	}
	if pendingTotal != 1 {
		t.Errorf("status=pending_verification matched %d, want 1", pendingTotal)
	}

	search := "listdonor"
	_, searchTotal, err := users.List(ctx, service.ListUserParams{Search: &search, Limit: 25})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if searchTotal != 1 {
		t.Errorf("search %q matched %d, want 1", search, searchTotal)
	}

	// An unknown filter value is a validation error, not silently ignored —
	// silently ignoring it would show an admin the wrong list without saying so.
	bad := "wizard"
	if _, _, err := users.List(ctx, service.ListUserParams{Role: &bad, Limit: 25}); !errors.Is(err, service.ErrInvalid) {
		t.Errorf("an unknown role filter = %v, want ErrInvalid", err)
	}
	if _, _, err := users.List(ctx, service.ListUserParams{Status: &bad, Limit: 25}); !errors.Is(err, service.ErrInvalid) {
		t.Errorf("an unknown status filter = %v, want ErrInvalid", err)
	}
}

// The bootstrap path's guard: it must run only when nobody can administer the
// deployment, so that leaving the env var set re-opens nothing on later boots.
func TestHasActiveAdminGatesTheBootstrap(t *testing.T) {
	users, _, pool := userSetup(t)
	ctx := context.Background()

	has, err := users.HasActiveAdmin(ctx)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if has {
		t.Fatal("an empty deployment reports an active admin")
	}

	id := testsupport.NewAdmin(t, pool, "first@example.test")
	if has, _ = users.HasActiveAdmin(ctx); !has {
		t.Fatal("an admin exists but is not reported")
	}

	// A suspended admin does not count: they cannot invite anyone, so a
	// deployment whose only admin is suspended still needs bootstrapping.
	if _, err := pool.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, id); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if has, _ = users.HasActiveAdmin(ctx); has {
		t.Error("a suspended admin counts as active")
	}
}

func TestGetUserNotFound(t *testing.T) {
	users, _, _ := userSetup(t)
	if _, err := users.Get(context.Background(), 999999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("get missing user = %v, want ErrNotFound", err)
	}
}
