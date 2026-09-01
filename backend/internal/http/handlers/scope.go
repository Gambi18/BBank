package handlers

import (
	"net/http"

	"bbank/internal/domain"
	"bbank/internal/middleware"
	"bbank/internal/service"
)

// resolveScope turns the scope granted by the RBAC middleware into the explicit
// filter the service takes (TRD §7.7).
//
// ok=false means the scope cannot be satisfied at all, and the caller must
// return an EMPTY list rather than an unfiltered one. That direction of failure
// is the entire point: a staff account with no home centre can see nothing, and
// the way that must fail is "no rows", never "all rows".
//
// This replaces the legacy `scopeClause`, which built the same rule by
// concatenating SQL fragments. Same logic, but a missing clause here is a nil
// pointer the service treats as "not narrowed" — so the decision is made once,
// in one place, instead of being spread across every query string.
func resolveScope(r *http.Request) (service.Scope, bool) {
	id, authed := middleware.IdentityFrom(r.Context())
	if !authed {
		return service.Scope{}, false
	}
	switch middleware.ScopeFrom(r.Context()) {
	case domain.ScopeAll:
		return service.Scope{}, true
	case domain.ScopeOwn:
		owner := id.UserID
		return service.Scope{OwnerID: &owner}, true
	case domain.ScopeCenter:
		if id.CenterID == nil {
			return service.Scope{}, false
		}
		center := *id.CenterID
		return service.Scope{CenterID: &center}, true
	}
	return service.Scope{}, false
}

// permitFunc adapts middleware.Permits to the callback the service uses to
// authorize a row it has locked.
//
// The check happens inside the service's transaction, against the row it is
// about to write, rather than against one read earlier and separately. That is
// what stops a request being authorized in one state and written in another.
func permitFunc(r *http.Request) func(ownerID, centerID int64) bool {
	return func(ownerID, centerID int64) bool {
		c := centerID
		return middleware.Permits(r.Context(), middleware.Row{OwnerID: ownerID, CenterID: &c})
	}
}
