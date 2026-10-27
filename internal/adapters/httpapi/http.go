package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
	"example.com/backend-golang-assignment/internal/service"
)

type App struct {
	users  *service.UserService
	tokens *service.TokenService
	logger *slog.Logger
}

func New(users *service.UserService, tokens *service.TokenService, logger *slog.Logger) http.Handler {
	a := &App{users: users, tokens: tokens, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/v1/auth/register", a.register)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.Handle("POST /api/v1/users", a.withAuth(http.HandlerFunc(a.register)))
	mux.Handle("GET /api/v1/users", a.withAuth(http.HandlerFunc(a.list)))
	mux.Handle("GET /api/v1/users/{id}", a.withAuth(http.HandlerFunc(a.get)))
	mux.Handle("PATCH /api/v1/users/{id}", a.withAuth(http.HandlerFunc(a.update)))
	mux.Handle("DELETE /api/v1/users/{id}", a.withAuth(http.HandlerFunc(a.delete)))
	return a.logging(mux)
}

type ctxKey struct{}

func currentUser(r *http.Request) string {
	v, _ := r.Context().Value(ctxKey{}).(string)
	return v
}

func (a *App) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		id, err := a.tokens.Verify(parts[1])
		if err != nil {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		// A deleted user must not continue using a previously issued access token.
		_, err = a.users.Get(r.Context(), id)
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (a *App) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		writer := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		a.logger.Info("http request", "method", r.Method, "path", r.URL.Path,
			"status", writer.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return domain.ErrInvalid
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return domain.ErrInvalid
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	code, status, message := "internal_error", http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		code, status, message = "invalid_input", 400, "invalid request"
	case errors.Is(err, domain.ErrUnauthorized):
		code, status, message = "unauthorized", 401, "authentication required or invalid"
	case errors.Is(err, domain.ErrForbidden):
		code, status, message = "forbidden", 403, "not allowed"
	case errors.Is(err, domain.ErrNotFound):
		code, status, message = "not_found", 404, "user not found"
	case errors.Is(err, domain.ErrConflict):
		code, status, message = "conflict", 409, "email already exists"
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var in service.RegisterInput
	if err := decode(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	u, err := a.users.Register(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/users/"+u.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"user": u})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	user, err := a.users.Authenticate(r.Context(), in.Email, in.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	token, err := a.tokens.Issue(user.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"access_token": token, "token_type": "Bearer"})
}
func (a *App) get(w http.ResponseWriter, r *http.Request) {
	u, err := a.users.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}
func (a *App) list(w http.ResponseWriter, r *http.Request) {
	page, err := parsePositive(r.URL.Query().Get("page"), 1)
	if err != nil {
		writeError(w, domain.ErrInvalid)
		return
	}
	limit, err := parsePositive(r.URL.Query().Get("limit"), 20)
	if err != nil || limit > 100 || page > 1000000 {
		writeError(w, domain.ErrInvalid)
		return
	}
	users, err := a.users.List(r.Context(), limit, (page-1)*limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "page": page, "limit": limit})
}
func parsePositive(raw string, defaultValue int64) (int64, error) {
	if raw == "" {
		return defaultValue, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid positive integer")
	}
	return n, nil
}
func (a *App) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !strings.EqualFold(currentUser(r), id) {
		writeError(w, domain.ErrForbidden)
		return
	}
	var in struct {
		Name  *string `json:"name"`
		Email *string `json:"email"`
	}
	if err := decode(w, r, &in); err != nil {
		writeError(w, err)
		return
	}
	u, err := a.users.Update(r.Context(), id, domain.UpdateFields{Name: in.Name, Email: in.Email})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}
func (a *App) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !strings.EqualFold(currentUser(r), id) {
		writeError(w, domain.ErrForbidden)
		return
	}
	if err := a.users.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
