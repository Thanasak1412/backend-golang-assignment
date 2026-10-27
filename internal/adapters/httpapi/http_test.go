package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
	"example.com/backend-golang-assignment/internal/service"
)

const uid = "507f1f77bcf86cd799439011"

type repoMock struct{ users map[string]domain.User }

func (m *repoMock) Create(_ context.Context, u domain.User) (domain.User, error) {
	for _, x := range m.users {
		if x.Email == u.Email {
			return domain.User{}, domain.ErrConflict
		}
	}
	u.ID = uid
	m.users[u.ID] = u
	return u, nil
}
func (m *repoMock) GetByID(_ context.Context, id string) (domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return u, domain.ErrNotFound
	}
	return u, nil
}
func (m *repoMock) GetByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNotFound
}
func (m *repoMock) List(_ context.Context, _, _ int64) ([]domain.User, error) {
	v := []domain.User{}
	for _, u := range m.users {
		v = append(v, u)
	}
	return v, nil
}
func (m *repoMock) Update(_ context.Context, id string, f domain.UpdateFields) (domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return u, domain.ErrNotFound
	}
	if f.Name != nil {
		u.Name = *f.Name
	}
	if f.Email != nil {
		u.Email = *f.Email
	}
	m.users[id] = u
	return u, nil
}
func (m *repoMock) Delete(_ context.Context, id string) error {
	if _, ok := m.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.users, id)
	return nil
}
func (m *repoMock) Count(_ context.Context) (int64, error) { return int64(len(m.users)), nil }
func request(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestHandlers(t *testing.T) {
	repo := &repoMock{users: map[string]domain.User{}}
	tokens, _ := service.NewTokenService("this_is_a_strong_32_byte_secret_012345", "test", time.Hour)
	h := New(service.NewUserService(repo), tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if w := request(h, "GET", "/healthz", "", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	registerBody := `{"name":"Alice","email":"Alice@EXAMPLE.COM","password":"password123"}`
	w := request(h, "POST", "/api/v1/auth/register", registerBody, "")
	if w.Code != 201 {
		t.Fatalf("registration %d: %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("password")) {
		t.Fatal("password leaked")
	}
	if w := request(h, "POST", "/api/v1/auth/register", registerBody, ""); w.Code != 409 {
		t.Fatalf("duplicate %d", w.Code)
	}
	if w := request(h, "GET", "/api/v1/users", "", ""); w.Code != 401 {
		t.Fatalf("unauthorized %d", w.Code)
	}
	login := request(h, "POST", "/api/v1/auth/login", `{"email":"alice@example.com","password":"password123"}`, "")
	if login.Code != 200 {
		t.Fatalf("login %d: %s", login.Code, login.Body.String())
	}
	var response map[string]string
	if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	tok := response["access_token"]
	if w := request(h, "GET", "/api/v1/users?page=1&limit=20", "", tok); w.Code != 200 {
		t.Fatalf("list %d", w.Code)
	}
	if w := request(h, "GET", "/api/v1/users/"+uid, "", tok); w.Code != 200 {
		t.Fatalf("get %d", w.Code)
	}
	if w := request(h, "GET", "/api/v1/users/not-an-objectid", "", tok); w.Code != 400 {
		t.Fatalf("bad id %d", w.Code)
	}
	if w := request(h, "PATCH", "/api/v1/users/000000000000000000000001", `{"name":"Bob"}`, tok); w.Code != 403 {
		t.Fatalf("must reject cross-user update: %d", w.Code)
	}
	if w := request(h, "PATCH", "/api/v1/users/"+uid, `{"name":"Bob"}`, tok); w.Code != 200 {
		t.Fatalf("update %d", w.Code)
	}
	if w := request(h, "DELETE", "/api/v1/users/"+uid, "", tok); w.Code != 204 {
		t.Fatalf("delete %d", w.Code)
	}
	if w := request(h, "GET", "/api/v1/users", "", tok); w.Code != 401 {
		t.Fatalf("deleted-user token accepted: %d", w.Code)
	}
}
func TestRejectUnknownJSON(t *testing.T) {
	repo := &repoMock{users: map[string]domain.User{}}
	tokens, _ := service.NewTokenService("this_is_a_strong_32_byte_secret_012345", "test", time.Hour)
	h := New(service.NewUserService(repo), tokens, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, body := range []string{`{"name":"A","email":"a@b.co","password":"12345678","role":"admin"}`, `{"name":"A","email":"a@b.co","password":"12345678"} {}`} {
		w := request(h, "POST", "/api/v1/auth/register", body, "")
		if w.Code != 400 {
			t.Fatalf("invalid json accepted: %d %s", w.Code, w.Body.String())
		}
	}
}
