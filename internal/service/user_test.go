package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
)

const testUserID = "507f1f77bcf86cd799439011"

type fakeRepo struct {
	users   map[string]domain.User
	nextErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[string]domain.User{}} }
func (r *fakeRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	if r.nextErr != nil {
		return domain.User{}, r.nextErr
	}
	for _, v := range r.users {
		if v.Email == u.Email {
			return domain.User{}, domain.ErrConflict
		}
	}
	u.ID = testUserID
	r.users[u.ID] = u
	return u, nil
}
func (r *fakeRepo) GetByID(_ context.Context, id string) (domain.User, error) {
	if r.nextErr != nil {
		return domain.User{}, r.nextErr
	}
	u, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}
func (r *fakeRepo) GetByEmail(_ context.Context, email string) (domain.User, error) {
	if r.nextErr != nil {
		return domain.User{}, r.nextErr
	}
	for _, v := range r.users {
		if v.Email == email {
			return v, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}
func (r *fakeRepo) List(_ context.Context, limit, offset int64) ([]domain.User, error) {
	if r.nextErr != nil {
		return nil, r.nextErr
	}
	all := []domain.User{}
	for _, v := range r.users {
		all = append(all, v)
	}
	if offset >= int64(len(all)) {
		return []domain.User{}, nil
	}
	end := offset + limit
	if end > int64(len(all)) {
		end = int64(len(all))
	}
	return all[offset:end], nil
}
func (r *fakeRepo) Update(_ context.Context, id string, f domain.UpdateFields) (domain.User, error) {
	if r.nextErr != nil {
		return domain.User{}, r.nextErr
	}
	u, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	if f.Name != nil {
		u.Name = *f.Name
	}
	if f.Email != nil {
		for other, v := range r.users {
			if other != id && v.Email == *f.Email {
				return domain.User{}, domain.ErrConflict
			}
		}
		u.Email = *f.Email
	}
	r.users[id] = u
	return u, nil
}
func (r *fakeRepo) Delete(_ context.Context, id string) error {
	if r.nextErr != nil {
		return r.nextErr
	}
	if _, ok := r.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.users, id)
	return nil
}
func (r *fakeRepo) Count(_ context.Context) (int64, error) { return int64(len(r.users)), r.nextErr }

func TestRegisterAndLogin(t *testing.T) {
	repo := newFakeRepo()
	svc := NewUserService(repo)
	user, err := svc.Register(context.Background(), RegisterInput{Name: " Alice ", Email: "ALICE@Example.COM", Password: "correct-password"})
	if err != nil {
		t.Fatal(err)
	}
	if user.Name != "Alice" || user.Email != "alice@example.com" || user.CreatedAt.IsZero() {
		t.Fatalf("bad user: %+v", user)
	}
	if user.PasswordHash == "correct-password" || user.PasswordHash == "" {
		t.Fatal("password was not hashed")
	}
	if strings.Contains(stringMustJSON(user), "password") {
		t.Fatal("password field leaked to response")
	}
	if _, err := svc.Authenticate(context.Background(), "alice@example.com", "correct-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), "alice@example.com", "wrong"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), "missing@example.com", "wrong"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("missing user: %v", err)
	}
	if _, err := svc.Register(context.Background(), RegisterInput{Name: "A", Email: "alice@example.com", Password: "longpassword"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate email: %v", err)
	}
}
func stringMustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestInvalidRegister(t *testing.T) {
	for _, in := range []RegisterInput{
		{Name: "", Email: "test@example.com", Password: "password123"},
		{Name: "Alice", Email: "bad-email", Password: "password123"},
		{Name: "Alice", Email: "test@example.com", Password: "short"},
		{Name: "Alice", Email: "test@example.com", Password: strings.Repeat("a", 73)},
	} {
		_, err := NewUserService(newFakeRepo()).Register(context.Background(), in)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("input %q expected invalid got %v", in.Email, err)
		}
	}
}
func TestCRUD(t *testing.T) {
	repo := newFakeRepo()
	s := NewUserService(repo)
	u, err := s.Register(context.Background(), RegisterInput{Name: "A", Email: "a@ex.com", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), "invalid"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	name := "Bob"
	updated, err := s.Update(context.Background(), u.ID, domain.UpdateFields{Name: &name})
	if err != nil || updated.Name != "Bob" {
		t.Fatalf("update: %v %v", updated, err)
	}
	if _, err := s.List(context.Background(), 101, 0); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("limit should be rejected")
	}
	if err := s.Delete(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), u.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
}
func TestLoginDatabaseError(t *testing.T) {
	repo := newFakeRepo()
	repo.nextErr = errors.New("database unavailable")
	_, err := NewUserService(repo).Authenticate(context.Background(), "a@b.com", "password")
	if err == nil || errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("must preserve database error: %v", err)
	}
}
func TestTimeUTC(t *testing.T) {
	repo := newFakeRepo()
	s := NewUserService(repo)
	s.now = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.FixedZone("UTC+7", 7*3600)) }
	u, err := s.Register(context.Background(), RegisterInput{Name: "A", Email: "a@b.com", Password: "password123"})
	if err != nil || u.CreatedAt.Location() != time.UTC {
		t.Fatalf("unexpected time: %+v %v", u, err)
	}
}
