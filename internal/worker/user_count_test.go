package worker

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
)

type countRepo struct{ calls atomic.Int64 }

func (*countRepo) Create(context.Context, domain.User) (domain.User, error) {
	return domain.User{}, nil
}
func (*countRepo) GetByID(context.Context, string) (domain.User, error)      { return domain.User{}, nil }
func (*countRepo) GetByEmail(context.Context, string) (domain.User, error)   { return domain.User{}, nil }
func (*countRepo) List(context.Context, int64, int64) ([]domain.User, error) { return nil, nil }
func (*countRepo) Update(context.Context, string, domain.UpdateFields) (domain.User, error) {
	return domain.User{}, nil
}
func (*countRepo) Delete(context.Context, string) error   { return nil }
func (r *countRepo) Count(context.Context) (int64, error) { return r.calls.Add(1), nil }
func TestCounterStopsOnCancel(t *testing.T) {
	repo := &countRepo{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		(UserCounter{Repo: repo, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Interval: time.Millisecond}).Run(ctx)
		close(done)
	}()
	deadline := time.After(300 * time.Millisecond)
	for repo.calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("counter did not tick")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("worker leaked")
	}
}
