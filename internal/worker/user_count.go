package worker

import (
	"context"
	"log/slog"
	"time"

	"example.com/backend-golang-assignment/internal/domain"
)

type UserCounter struct {
	Repo     domain.UserRepository
	Logger   *slog.Logger
	Interval time.Duration
}

// Run executes sequential counts: slow queries cannot accumulate goroutines.
func (w UserCounter) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			count, err := w.Repo.Count(queryCtx)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					w.Logger.Error("count users failed", "error", err)
				}
				continue
			}
			w.Logger.Info("users total", "count", count)
		}
	}
}
