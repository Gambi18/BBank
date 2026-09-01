package platform

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

var idEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// newID returns a prefixed, unguessable identifier: "ses_01ARZ3NDEK...".
//
// Prefixed so an id is self-describing in a log or a bug report, and random
// rather than sequential so a public identifier never leaks row counts or
// allows enumeration.
func newID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + "_" + strings.ToLower(idEncoding.EncodeToString(b))
}

// NewID is the exported form for callers outside this package.
func NewID(prefix string) string { return newID(prefix) }

// NewOpaqueToken returns a 256-bit random token for refresh cookies.
func NewOpaqueToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return strings.ToLower(idEncoding.EncodeToString(b))
}
