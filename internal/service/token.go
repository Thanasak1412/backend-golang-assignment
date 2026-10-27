package service

import (
	"fmt"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
	"github.com/golang-jwt/jwt/v5"
)

type TokenService struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenService(secret, issuer string, ttl time.Duration) (*TokenService, error) {
	if len(secret) < 32 || issuer == "" || ttl <= 0 {
		return nil, fmt.Errorf("jwt requires a >=32-byte secret, issuer and positive ttl")
	}
	return &TokenService{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}, nil
}

func (t *TokenService) Issue(userID string) (string, error) {
	if userID == "" {
		return "", domain.ErrInvalid
	}
	now := t.now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    t.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *TokenService) Verify(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(t.issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return "", domain.ErrUnauthorized
	}
	if err := validateID(claims.Subject); err != nil {
		return "", domain.ErrUnauthorized
	}
	return claims.Subject, nil
}
