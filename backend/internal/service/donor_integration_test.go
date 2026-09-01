package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"golang.org/x/crypto/bcrypt"
)

func baseCreate(email string) service.CreateParams {
	return service.CreateParams{
		Email:       email,
		Password:    "correct-horse-battery",
		FullName:    "New Donor",
		DateOfBirth: time.Date(1994, 6, 1, 0, 0, 0, 0, time.UTC),
		Gender:      "female",
		Phone:       "+237600333444",
	}
}

// Registration writes users + donor_profiles in one statement, so there is
// never a window in which a user exists without a profile — the state that made
// createRequest fail on a foreign key and return 500 for a missing profile.
func TestCreateDonorWritesBothRows(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))

	id, err := svc.Create(context.Background(), baseCreate("newdonor@example.test"), false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM users WHERE id = $1 AND role = 'donor'`, id); n != 1 {
		t.Errorf("users row missing")
	}
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM donor_profiles WHERE user_id = $1`, id); n != 1 {
		t.Errorf("donor_profiles row missing")
	}
}

// The password must be stored as a bcrypt hash, never as anything a reader
// could use. This is the defect the project started with.
func TestCreateDonorStoresABcryptHashNotThePassword(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))

	const password = "correct-horse-battery"
	id, err := svc.Create(context.Background(), baseCreate("hash@example.test"), false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var hash string
	if err := pool.QueryRow(context.Background(),
		`SELECT password_hash FROM users WHERE id = $1`, id).Scan(&hash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if hash == password || strings.Contains(hash, password) {
		t.Fatal("the password is recoverable from the stored value")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("stored value %q is not a bcrypt hash", hash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Errorf("the stored hash does not verify the original password: %v", err)
	}
}

// Uniqueness on users.email is the only thing that holds under concurrent
// signups; checking first and trusting the answer is a race. Driving it
// concurrently is the only way to test that claim.
func TestDuplicateEmailIsAConflictEvenUnderRace(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))

	const attempts = 6
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, conflicts := 0, 0
	var other []error
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Create(context.Background(), baseCreate("racer@example.test"), false)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case errors.Is(err, service.ErrConflict):
				conflicts++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("a duplicate email surfaced as something other than a conflict: %v", other)
	}
	if created != 1 || conflicts != attempts-1 {
		t.Errorf("created=%d conflicts=%d, want 1 and %d", created, conflicts, attempts-1)
	}
}

// FR-21: blood group is a laboratory result. A self-registering donor may not
// assert it; staff may. The original system let people type it, which is how
// "O+", "o" and " A " ended up in one column (D7).
func TestBloodGroupIsNotSelfReportable(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))
	ctx := context.Background()

	group, rh := "O", "negative"

	selfClaim := baseCreate("selfclaim@example.test")
	selfClaim.BloodGroup, selfClaim.Rhesus = &group, &rh
	selfID, err := svc.Create(ctx, selfClaim, false) // allowClinical = false
	if err != nil {
		t.Fatalf("self registration: %v", err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM donor_profiles WHERE user_id = $1 AND blood_group IS NOT NULL`, selfID); n != 0 {
		t.Error("a self-registering donor set their own blood group")
	}

	byStaff := baseCreate("bystaff@example.test")
	byStaff.BloodGroup, byStaff.Rhesus = &group, &rh
	staffID, err := svc.Create(ctx, byStaff, true) // allowClinical = true
	if err != nil {
		t.Fatalf("staff registration: %v", err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM donor_profiles WHERE user_id = $1 AND blood_group = 'O' AND rhesus = 'negative'`, staffID); n != 1 {
		t.Error("staff could not record a blood group")
	}
}

// A profile edit that omits the clinical fields must not blank them. Silently
// erasing a recorded blood type because a form did not include the field would
// be worse than refusing the edit.
func TestUpdateCarriesClinicalValuesForward(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))
	ctx := context.Background()

	group, rh := "AB", "positive"
	p := baseCreate("carry@example.test")
	p.BloodGroup, p.Rhesus = &group, &rh
	id, err := svc.Create(ctx, p, true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// A donor editing their own profile: no clinical fields, not permitted anyway.
	err = svc.Update(ctx, id, service.UpdateParams{
		FullName: "Renamed Donor", Gender: "female", Phone: "+237600999888",
		DateOfBirth: time.Date(1994, 6, 1, 0, 0, 0, 0, time.UTC),
	}, false)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	var name, bg, rhesus string
	if err := pool.QueryRow(ctx,
		`SELECT full_name, blood_group::text, rhesus::text FROM donor_profiles WHERE user_id = $1`, id).
		Scan(&name, &bg, &rhesus); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if name != "Renamed Donor" {
		t.Errorf("the edit did not apply: name = %q", name)
	}
	if bg != "AB" || rhesus != "positive" {
		t.Errorf("the recorded blood type was lost: got %s/%s, want AB/positive", bg, rhesus)
	}
}

// "unknown" is not a gender. The enum is male|female|other|undisclosed, and
// guessing otherwise broke every signup once already.
func TestInvalidGenderIsRejectedBeforeItReachesPostgres(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))

	p := baseCreate("badgender@example.test")
	p.Gender = "unknown"

	_, err := svc.Create(context.Background(), p, false)
	if !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("gender %q = %v, want ErrInvalid (a 422 naming the field, not a 500)", p.Gender, err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM users WHERE email = 'badgender@example.test'`); n != 0 {
		t.Error("a rejected registration still created a user")
	}
}

// An empty gender means "not asked", which is `undisclosed` — not an error and
// not a blank.
func TestUnstatedGenderBecomesUndisclosed(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))

	p := baseCreate("nogender@example.test")
	p.Gender = ""
	id, err := svc.Create(context.Background(), p, false)
	if err != nil {
		t.Fatalf("create with no gender: %v", err)
	}
	var gender string
	if err := pool.QueryRow(context.Background(),
		`SELECT gender::text FROM donor_profiles WHERE user_id = $1`, id).Scan(&gender); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gender != "undisclosed" {
		t.Errorf("gender = %q, want undisclosed", gender)
	}
}

func TestShortPasswordIsRejected(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))

	p := baseCreate("shortpw@example.test")
	p.Password = "abc"
	if _, err := svc.Create(context.Background(), p, false); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("short password = %v, want ErrInvalid", err)
	}
}

// Donor reads: the list is bounded and searchable, and eligibility is a read of
// computed state with no setter — it comes from real donation records plus
// deferrals plus policy, never from a field anyone types (defect D4).
func TestDonorListAndEligibility(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonorService(store.New(pool))
	ctx := context.Background()

	for _, n := range []string{"Ada Search", "Grace Search", "Unrelated Person"} {
		if _, err := svc.Create(ctx, service.CreateParams{
			Email:    strings.ToLower(strings.ReplaceAll(n, " ", ".")) + "@example.test",
			Password: "correct-horse-battery", FullName: n, Gender: "other",
			DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC), Phone: "+237600000001",
		}, false); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}

	all, total, err := svc.List(ctx, service.ListParams{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Errorf("list returned %d rows / total %d, want 3", len(all), total)
	}

	search := "Search"
	found, foundTotal, err := svc.List(ctx, service.ListParams{Search: &search})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if foundTotal != 2 || len(found) != 2 {
		t.Errorf("search %q returned %d / total %d, want 2", search, len(found), foundTotal)
	}

	// An unbounded list is defect A15. Zero means "unset" and must become the
	// default, never "no limit".
	page, _, err := svc.List(ctx, service.ListParams{Limit: 2})
	if err != nil {
		t.Fatalf("paged list: %v", err)
	}
	if len(page) != 2 {
		t.Errorf("limit=2 returned %d rows", len(page))
	}

	elig, err := svc.Eligibility(ctx, all[0].ID)
	if err != nil {
		t.Fatalf("eligibility: %v", err)
	}
	if elig.DonorID != all[0].ID {
		t.Errorf("eligibility is for donor %d, asked for %d", elig.DonorID, all[0].ID)
	}

	if _, err := svc.Eligibility(ctx, 999999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("eligibility for a missing donor = %v, want ErrNotFound", err)
	}
	if _, err := svc.Get(ctx, 999999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("get missing donor = %v, want ErrNotFound", err)
	}
	if err := svc.Update(ctx, 999999, service.UpdateParams{FullName: "Ghost", Gender: "other"}, false); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("update missing donor = %v, want ErrNotFound", err)
	}
}
