package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"example.com/backend-golang-assignment/internal/domain"
)

var objectIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{24}$`)

// UserService has no dependency on HTTP or MongoDB.
type UserService struct {
	repo domain.UserRepository
	now  func() time.Time
}

func NewUserService(repo domain.UserRepository) *UserService {
	return &UserService{repo: repo, now: time.Now}
}

type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func validateName(v string) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) < 1 || utf8.RuneCountInString(v) > 100 {
		return "", fmt.Errorf("name must be 1-100 characters: %w", domain.ErrInvalid)
	}
	return v, nil
}

func normalizeEmail(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) == 0 || len(v) > 254 || strings.ContainsAny(v, " \t\n\r") {
		return "", fmt.Errorf("invalid email: %w", domain.ErrInvalid)
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || !strings.Contains(v, "@") {
		return "", fmt.Errorf("invalid email: %w", domain.ErrInvalid)
	}
	return v, nil
}

func validateID(id string) error {
	if !objectIDPattern.MatchString(id) {
		return fmt.Errorf("invalid user id: %w", domain.ErrInvalid)
	}
	return nil
}

func (s *UserService) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	name, err := validateName(in.Name)
	if err != nil {
		return domain.User{}, err
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return domain.User{}, err
	}
	// bcrypt cannot safely hash passwords longer than 72 bytes.
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return domain.User{}, fmt.Errorf("password must be 8-72 bytes: %w", domain.ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	return s.repo.Create(ctx, domain.User{
		Name: name, Email: email, PasswordHash: string(hash), CreatedAt: s.now().UTC(),
	})
}

func (s *UserService) Get(ctx context.Context, id string) (domain.User, error) {
	if err := validateID(id); err != nil {
		return domain.User{}, err
	}
	return s.repo.GetByID(ctx, strings.ToLower(id))
}

func (s *UserService) List(ctx context.Context, limit, offset int64) ([]domain.User, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, domain.ErrInvalid
	}
	return s.repo.List(ctx, limit, offset)
}

func (s *UserService) Update(ctx context.Context, id string, fields domain.UpdateFields) (domain.User, error) {
	if err := validateID(id); err != nil {
		return domain.User{}, err
	}
	if fields.Name == nil && fields.Email == nil {
		return domain.User{}, domain.ErrInvalid
	}
	if fields.Name != nil {
		name, err := validateName(*fields.Name)
		if err != nil {
			return domain.User{}, err
		}
		fields.Name = &name
	}
	if fields.Email != nil {
		email, err := normalizeEmail(*fields.Email)
		if err != nil {
			return domain.User{}, err
		}
		fields.Email = &email
	}
	return s.repo.Update(ctx, strings.ToLower(id), fields)
}

func (s *UserService) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, strings.ToLower(id))
}

func (s *UserService) Count(ctx context.Context) (int64, error) { return s.repo.Count(ctx) }

func (s *UserService) Authenticate(ctx context.Context, email, password string) (domain.User, error) {
	// Authentication deliberately returns the same error for unknown users and wrong passwords.
	canonical, err := normalizeEmail(email)
	if err != nil {
		return domain.User{}, domain.ErrUnauthorized
	}
	u, err := s.repo.GetByEmail(ctx, canonical)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return domain.User{}, domain.ErrUnauthorized
	}
	return u, nil
}
