package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	grpcapp "voice-chat-sfu/internal/app/grpc"
	"voice-chat-sfu/internal/config"
	"voice-chat-sfu/internal/lib/logger"
)

type App struct {
	container  *diContainer
	gRPCServer *grpcapp.App
	cfg        *config.Config
	log        *slog.Logger
}

func New() (*App, error) {
	const op = "App.New"

	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	container := newDIContainer(cfg, log)
	gRPCServer, err := container.GRPCApp()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &App{
		container:  container,
		gRPCServer: gRPCServer,
		cfg:        cfg,
		log:        log,
	}, nil
}

func (a *App) Run() error {
	const op = "App.Run"

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	a.log.Info("starting gRPC server", "port", a.cfg.GRPCServer.Port)

	go func() {
		errCh <- a.gRPCServer.Run()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		return nil
	case <-ctx.Done():
		a.log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ServerShutdownTimeout())
	defer cancel()

	if err := a.gRPCServer.Close(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := <-errCh; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	a.log.Info("server stopped")

	return nil
}
