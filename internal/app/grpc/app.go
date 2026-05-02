package grpcapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"
	"voice-chat-sfu/internal/grpc/session"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type App struct {
	log        *slog.Logger
	gRPCServer *grpc.Server
	port       int
}

type Config struct {
	Port                int
	KeepaliveTime       time.Duration
	KeepaliveTimeout    time.Duration
	KeepaliveMinTime    time.Duration
	PermitWithoutStream bool
}

func New(
	log *slog.Logger,
	cfg Config,
	sessionCfg session.Config,
	sessionService session.SessionService,
) *App {
	gRPCServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    cfg.KeepaliveTime,
			Timeout: cfg.KeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             cfg.KeepaliveMinTime,
			PermitWithoutStream: cfg.PermitWithoutStream,
		}),
	)
	session.Register(gRPCServer, sessionService, sessionCfg)

	return &App{
		log:        log,
		gRPCServer: gRPCServer,
		port:       cfg.Port,
	}
}

func (a *App) Run() error {
	const op = "grpcapp.App.Run"

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", a.port))
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	a.log.Info("gRPC server listening", "port", a.port)

	if err := a.gRPCServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (a *App) Close(ctx context.Context) error {
	const op = "grpcapp.App.Close"

	done := make(chan struct{})

	go func() {
		a.gRPCServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		a.log.Warn("graceful shutdown timed out, forcing stop")
		a.gRPCServer.Stop()
		<-done
		return fmt.Errorf("%s: %w", op, ctx.Err())
	}
}
