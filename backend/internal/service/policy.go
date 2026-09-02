package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"bbank/internal/domain"
	"bbank/internal/store"
)

// PolicyTTL is how long a resolved snapshot is reused before it is re-read.
//
// A minute is a compromise between two real costs. Reading `active_policies` on
// every eligibility decision puts a query in front of every booking and every
// check-in for a table that changes a few times a year. Caching it forever
// means an administrator who corrects a threshold — the point of `FR-20` being
// data rather than code — waits for a redeploy to see it take effect, which is
// the thing this whole mechanism exists to avoid.
//
// A minute also bounds the blast radius of a bad edit: a wrong number is live
// for at most a minute after it is corrected, not until the next release.
const PolicyTTL = time.Minute

// defaultPolicyRegion is the wildcard every seeded row uses (schema §12.1).
const defaultPolicyRegion = "*"

// PolicyService resolves `active_policies` into an immutable snapshot.
//
// The snapshot is the unit of consistency: every decision is made against one,
// and carries its version. See `queries/policies.sql` for why the whole set is
// read at once rather than key by key.
type PolicyService struct {
	q *store.Queries

	mu     sync.RWMutex
	cached *domain.Policies
	loaded time.Time
}

func NewPolicyService(q *store.Queries) *PolicyService {
	return &PolicyService{q: q}
}

// Current returns a snapshot, re-reading it if the cached one has aged out.
//
// It never returns a stale snapshot on error. If the reload fails, the error is
// returned and the caller cannot decide — which is the same stance
// `ErrPolicyMissing` takes, and for the same reason: a clinical decision made
// against numbers the system is no longer sure of is worse than no decision.
func (s *PolicyService) Current(ctx context.Context) (*domain.Policies, error) {
	s.mu.RLock()
	cached, loaded := s.cached, s.loaded
	s.mu.RUnlock()
	if cached != nil && time.Since(loaded) < PolicyTTL {
		return cached, nil
	}

	fresh, err := s.load(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cached, s.loaded = fresh, time.Now()
	s.mu.Unlock()
	return fresh, nil
}

// Reload discards the cache and reads immediately. For the policy console
// (`WI-89`) to call after an edit, so an administrator sees their change take
// effect rather than waiting out the TTL.
func (s *PolicyService) Reload(ctx context.Context) (*domain.Policies, error) {
	fresh, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cached, s.loaded = fresh, time.Now()
	s.mu.Unlock()
	return fresh, nil
}

func (s *PolicyService) load(ctx context.Context) (*domain.Policies, error) {
	rows, err := s.q.ListActivePolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("read active policies: %w", err)
	}

	// Only the default region, deliberately.
	//
	// `policies.region` exists so a future region can override a threshold, and
	// the exclusion constraint lets a `'*'` row and a region row coexist for one
	// key. But nothing in this codebase can yet SAY which region a decision is
	// for — there is no region on a request, a centre or a user — so there is no
	// honest way to pick between them.
	//
	// The first version of this resolved "a region row beats the default", which
	// would have applied the first region's age band to every decision
	// everywhere, and dropped a second region's row by query order. Skipping
	// them is the conservative reading: an unusable row is ignored rather than
	// misapplied, and `WI-89` makes the region a parameter when there is
	// something to parameterise it with.
	best := make(map[domain.PolicyKey]store.ActivePolicy, len(rows))
	skipped := 0
	for _, r := range rows {
		if r.Region != defaultPolicyRegion {
			skipped++
			continue
		}
		best[domain.PolicyKey(r.Key)] = r
	}
	if skipped > 0 {
		slog.WarnContext(ctx, "region-scoped policies ignored",
			"count", skipped,
			"reason", "no request carries a region yet; see WI-89")
	}
	fingerprintRows := make([]domain.PolicyRow, 0, len(best))

	raw := make(map[domain.PolicyKey]json.RawMessage, len(best))
	for key, r := range best {
		raw[key] = json.RawMessage(r.Value)
		fingerprintRows = append(fingerprintRows, domain.PolicyRow{
			Key: key, Region: r.Region, Value: json.RawMessage(r.Value),
		})
	}

	// The version fingerprints the rows ACTUALLY USED, not every row read. Two
	// snapshots that resolve to the same numbers are the same version even if
	// an unused region row was added between them — the version answers "which
	// numbers produced this decision", and an unused row produced nothing.
	return domain.NewPolicies(domain.PolicyVersion(fingerprintRows), raw), nil
}
