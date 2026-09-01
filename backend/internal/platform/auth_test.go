package platform

import (
	"strings"
	"testing"
	"time"
)

func newTestSigner(t *testing.T) *Signer {
	t.Helper()
	priv, _, err := GenerateKeyPEM()
	if err != nil {
		t.Fatalf("GenerateKeyPEM: %v", err)
	}
	s, err := NewSigner(priv, "https://api.bbank.test", "bbank-web", false)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return s
}

func subject() TokenSubject {
	cid := int64(7)
	return TokenSubject{UserID: 42, SessionID: "ses_x", Role: "lab_tech", CenterID: &cid, TokenVersion: 3}
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	s := newTestSigner(t)
	tok, exp, err := s.SignAccessToken(subject(), time.Now())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if time.Until(exp) > AccessTokenTTL+time.Second {
		t.Errorf("expiry %v exceeds the 15 minute TTL", time.Until(exp))
	}
	c, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if c.Subject != "42" || c.Role != "lab_tech" || c.TokenVersion != 3 {
		t.Errorf("claims round-tripped wrong: %+v", c)
	}
	if c.CenterID == nil || *c.CenterID != 7 {
		t.Error("cid claim lost — staff would lose their center scope")
	}
}

// A token edited by one byte must not verify. This is the property proxy.ts
// depends on, and the whole reason for replacing the unsigned JSON cookie.
func TestTamperedTokenIsRejected(t *testing.T) {
	s := newTestSigner(t)
	tok, _, _ := s.SignAccessToken(subject(), time.Now())

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	// Flip a character in the payload.
	payload := []byte(parts[1])
	if payload[5] == 'A' {
		payload[5] = 'B'
	} else {
		payload[5] = 'A'
	}
	tampered := parts[0] + "." + string(payload) + "." + parts[2]

	if _, err := s.Verify(tampered); err == nil {
		t.Fatal("a tampered token verified — the signature is not being checked")
	}
}

// A token signed by a different key must not verify against ours.
func TestForeignKeyIsRejected(t *testing.T) {
	mine, theirs := newTestSigner(t), newTestSigner(t)
	tok, _, _ := theirs.SignAccessToken(subject(), time.Now())
	if _, err := mine.Verify(tok); err == nil {
		t.Fatal("a token signed by another key verified")
	}
}

func TestExpiredTokenIsRejected(t *testing.T) {
	s := newTestSigner(t)
	tok, _, _ := s.SignAccessToken(subject(), time.Now().Add(-2*AccessTokenTTL))
	_, err := s.Verify(tok)
	if err == nil {
		t.Fatal("an expired token verified")
	}
	if err != ErrTokenExpired {
		t.Errorf("got %v, want ErrTokenExpired so callers can trigger a refresh", err)
	}
}

// Wrong issuer or audience must fail: a token minted for another service or
// another client must not be accepted here.
func TestIssuerAndAudienceAreChecked(t *testing.T) {
	priv, _, _ := GenerateKeyPEM()
	other, _ := NewSigner(priv, "https://evil.example", "bbank-web", false)
	mine, _ := NewSigner(priv, "https://api.bbank.test", "bbank-web", false)
	tok, _, _ := other.SignAccessToken(subject(), time.Now())
	if _, err := mine.Verify(tok); err == nil {
		t.Fatal("a token with a foreign issuer verified")
	}
}

// The alg-confusion attack: a token whose header claims "none" or HS256 must be
// refused, never verified using the public key as an HMAC secret.
func TestAlgConfusionIsRejected(t *testing.T) {
	s := newTestSigner(t)
	// alg:none with the same claims shape.
	none := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiI0MiIsInJvbGUiOiJhZG1pbiJ9."
	if _, err := s.Verify(none); err == nil {
		t.Fatal("an alg:none token verified — this is a full auth bypass")
	}
}

func TestSignerRejectsNonECKeys(t *testing.T) {
	if _, err := NewSigner("not a pem", "i", "a", false); err == nil {
		t.Error("garbage PEM was accepted")
	}
	if _, err := NewSigner("", "i", "a", false); err != ErrNoSigningKey {
		t.Error("an empty key should be ErrNoSigningKey when generation is not allowed")
	}
}

func TestPublicKeyPEMIsPublicOnly(t *testing.T) {
	s := newTestSigner(t)
	pub, err := s.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}
	if !strings.Contains(pub, "BEGIN PUBLIC KEY") {
		t.Error("expected a PUBLIC KEY block")
	}
	if strings.Contains(pub, "PRIVATE") {
		t.Fatal("the private key leaked into PublicKeyPEM — the frontend would be able to mint tokens")
	}
}
