package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ES256 (ECDSA P-256), not HS256, and the reason matters.
//
// proxy.ts in the Next.js frontend must VERIFY the access token on every
// navigation. With HS256 the verifying key is also the signing key, so the
// frontend would hold the power to MINT tokens — a frontend compromise would
// become an admin-token factory. ES256 gives Next.js only the public key.
//
// P-256 is chosen over Ed25519 because Web Crypto supports it in both the Node
// and Edge runtimes, which is what `jose` uses in proxy.ts.

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 7 * 24 * time.Hour
)

var (
	ErrNoSigningKey     = errors.New("JWT_PRIVATE_KEY is not set")
	ErrTokenInvalid     = errors.New("token is invalid")
	ErrTokenExpired     = errors.New("token has expired")
	ErrTokenVerMismatch = errors.New("token version is stale; credentials or role changed")
)

// Claims is the access-token payload (TRD §7.3).
type Claims struct {
	SessionID    string `json:"sid"`
	Role         string `json:"role"`
	CenterID     *int64 `json:"cid"`
	HospitalID   *int64 `json:"hid"`
	TokenVersion int32  `json:"ver"`
	jwt.RegisteredClaims
}

type Signer struct {
	priv     *ecdsa.PrivateKey
	pub      *ecdsa.PublicKey
	issuer   string
	audience string
}

// NewSigner loads the ES256 keypair. If JWT_PRIVATE_KEY is absent AND
// allowGenerate is set (development only), an ephemeral key is generated — which
// means every restart invalidates all tokens. That is deliberately noisy: it is
// not something you want to discover in production.
func NewSigner(pemKey, issuer, audience string, allowGenerate bool) (*Signer, error) {
	if pemKey == "" {
		if !allowGenerate {
			return nil, ErrNoSigningKey
		}
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ephemeral key: %w", err)
		}
		return &Signer{priv: key, pub: &key.PublicKey, issuer: issuer, audience: audience}, nil
	}

	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("JWT_PRIVATE_KEY is not valid PEM")
	}
	var key *ecdsa.PrivateKey
	switch block.Type {
	case "EC PRIVATE KEY":
		k, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse EC private key: %w", err)
		}
		key = k
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8 key: %w", err)
		}
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("JWT_PRIVATE_KEY is not an ECDSA key (ES256 requires P-256)")
		}
		key = ec
	default:
		return nil, fmt.Errorf("unsupported PEM block %q", block.Type)
	}
	if key.Curve != elliptic.P256() {
		return nil, errors.New("ES256 requires the P-256 curve")
	}
	return &Signer{priv: key, pub: &key.PublicKey, issuer: issuer, audience: audience}, nil
}

// PublicKeyPEM is what the frontend gets. Never the private key.
func (s *Signer) PublicKeyPEM() (string, error) {
	der, err := x509.MarshalPKIXPublicKey(s.pub)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

type TokenSubject struct {
	UserID       int64
	SessionID    string
	Role         string
	CenterID     *int64
	HospitalID   *int64
	TokenVersion int32
}

func (s *Signer) SignAccessToken(sub TokenSubject, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	claims := Claims{
		SessionID:    sub.SessionID,
		Role:         sub.Role,
		CenterID:     sub.CenterID,
		HospitalID:   sub.HospitalID,
		TokenVersion: sub.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience},
			Subject:   fmt.Sprintf("%d", sub.UserID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        newID("jti"),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	signed, err := tok.SignedString(s.priv)
	return signed, exp, err
}

// Verify checks signature, expiry, issuer and audience. It deliberately does NOT
// check token_version — that needs a database read, so it belongs in the
// middleware that already has one, not in the pure crypto path.
func (s *Signer) Verify(token string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			// Guards the alg-confusion attack: a token claiming HS256 must never
			// be verified with the public key as an HMAC secret.
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return s.pub, nil
	}, jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}
	return &c, nil
}

// GenerateKeyPEM prints a fresh keypair, for `make keys` / first deploy.
func GenerateKeyPEM() (privPEM string, pubPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	pder, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pder}))
	return privPEM, pubPEM, nil
}
