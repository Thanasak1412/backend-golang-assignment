package service

import (
	"errors"
	"testing"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWT(t *testing.T) {
	s, err := NewTokenService("a_very_long_secret_that_has_32_or_more_bytes", "unit-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Issue(testUserID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Verify(token)
	if err != nil || got != testUserID {
		t.Fatalf("token invalid: %s %v", got, err)
	}
	if _, err := s.Verify(token + "tamper"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("tampering accepted: %v", err)
	}
	s.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expired, _ := s.Issue(testUserID)
	if _, err := s.Verify(expired); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired accepted: %v", err)
	}
	// Reject tokens signed by a different HMAC algorithm even when the secret matches.
	bad := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.RegisteredClaims{
		Subject: testUserID, Issuer: "unit-test", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, _ := bad.SignedString(s.secret)
	if _, err := s.Verify(signed); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("HS384 accepted: %v", err)
	}
}
func TestJWTConfig(t *testing.T) {
	if _, err := NewTokenService("short", "test", time.Hour); err == nil {
		t.Fatal("weak secret accepted")
	}
}
