package grpcapp

import (
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
) *App {
	gRPCServer := grpc.NewServer()

	session.Register(gRPCServer)

	return &App{
		log:        log,
		gRPCServer: gRPCServer,
		port:       port,
	}
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil {
		panic(err)
	}
}

func (a *App) Run() error {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", a.port))
	if err != nil {
		return fmt.Errorf("") //TODO
	}

	if err := a.gRPCServer.Serve(l); err != nil {
		return fmt.Errorf("") //TODO
	}

	return nil
}

func (a *App) Stop() {
	a.gRPCServer.GracefulStop()
}
