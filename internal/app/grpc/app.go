package grpcapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"voice-chat-sfu/internal/grpc/session"

	"google.golang.org/grpc"
)

type App struct {
	log        *slog.Logger
	gRPCServer *grpc.Server
	port       int
}

func New(
	log *slog.Logger,
	port int,
	sessionService session.SessionService,
) *App {
	gRPCServer := grpc.NewServer()
	session.Register(gRPCServer, sessionService)

	return &App{
		log:        log,
		gRPCServer: gRPCServer,
		port:       port,
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
