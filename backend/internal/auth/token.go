// Package auth issues and verifies the HMAC-SHA256 JWTs used to protect the
// API. Keeping both sides in one package means the signing method, issuer and
// claim set can never drift apart between the token endpoint and the
// middleware that validates them.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

// signingMethod is fixed to HS256. Pinning it when parsing is what prevents
// algorithm confusion attacks, where a token forged with alg "none" or an
// asymmetric algorithm would otherwise be accepted.
var signingMethod = jwt.SigningMethodHS256

// TokenService issues and verifies access tokens.
type TokenService struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenService builds a token service. The secret is never logged or
// returned; it lives only in this struct.
func NewTokenService(secret, issuer string, ttl time.Duration) *TokenService {
	return &TokenService{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
		now:    time.Now,
	}
}

// Issue mints a short-lived token for the given subject and reports its
// lifetime in seconds.
func (s *TokenService) Issue(subject string) (string, int, error) {
	issuedAt := s.now().UTC()

	tokenID, err := randomID()
	if err != nil {
		return "", 0, domain.NewInternalError("Failed to generate a token identifier.")
	}

	claims := jwt.RegisteredClaims{
		Issuer:    s.issuer,
		Subject:   subject,
		ID:        tokenID,
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		NotBefore: jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(issuedAt.Add(s.ttl)),
	}

	signed, err := jwt.NewWithClaims(signingMethod, claims).SignedString(s.secret)
	if err != nil {
		return "", 0, domain.NewInternalError("Failed to sign the access token.")
	}
	return signed, int(s.ttl.Seconds()), nil
}

// Verify parses and validates a token, returning its subject. Every failure is
// reported as an unauthorized domain error, with a detail specific enough to
// debug against but free of cryptographic material.
func (s *TokenService) Verify(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}

	parsed, err := jwt.ParseWithClaims(token, claims,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)

	switch {
	case err == nil && parsed.Valid:
		return claims.Subject, nil
	case errors.Is(err, jwt.ErrTokenExpired):
		return "", domain.NewUnauthorizedError("The access token has expired.")
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return "", domain.NewUnauthorizedError("The access token is not valid yet.")
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return "", domain.NewUnauthorizedError("The access token signature is invalid.")
	case errors.Is(err, jwt.ErrTokenInvalidIssuer):
		return "", domain.NewUnauthorizedError("The access token was issued by an unknown issuer.")
	default:
		return "", domain.NewUnauthorizedError("The access token is malformed or invalid.")
	}
}

// randomID produces a 128-bit token identifier, allowing individual tokens to
// be traced in logs without exposing the token itself.
func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
