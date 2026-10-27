package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"example.com/backend-golang-assignment/internal/adapters/httpapi"
	mongorepo "example.com/backend-golang-assignment/internal/adapters/mongodb"
	"example.com/backend-golang-assignment/internal/config"
	"example.com/backend-golang-assignment/internal/service"
	"example.com/backend-golang-assignment/internal/worker"
)

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		cancel()
		return err
	}
	if err = client.Ping(connCtx, readpref.Primary()); err != nil {
		cancel()
		_ = client.Disconnect(context.Background())
		return err
	}
	cancel()
	defer func() {
		dc, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = client.Disconnect(dc)
	}()
	repo := mongorepo.NewRepository(client.Database(cfg.MongoDB).Collection("users"))
	indexCtx, indexCancel := context.WithTimeout(ctx, 10*time.Second)
	err = repo.EnsureIndexes(indexCtx)
	indexCancel()
	if err != nil {
		return err
	}
	users := service.NewUserService(repo)
	tokens, err := service.NewTokenService(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTDuration)
	if err != nil {
		return err
	}
	api := httpapi.New(users, tokens, logger)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		(worker.UserCounter{Repo: repo, Logger: logger, Interval: 10 * time.Second}).Run(ctx)
	}()
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	logger.Info("server listening", "address", cfg.HTTPAddr)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serverErr:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("http server failed", "error", serveErr)
		}
		stop()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	workerWG.Wait()
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return shutdownErr
}

func main() {
	if err := run(); err != nil {
		slog.Error("application exited", "error", err)
		os.Exit(1)
	}
}
