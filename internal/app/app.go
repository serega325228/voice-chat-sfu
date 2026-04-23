package app

import (
	"context"
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
}

func New() *App {
	cfg := config.MustLoad()

	log := logger.SetupLogger(cfg.Env)

	a := &App{
		container: newDIContainer(cfg, log),
		cfg:       cfg,
	}

	a.initDeps()

	return a
}

func (a *App) initDeps() {
	ctx := context.Background()

	a.container.Storage(ctx)
	a.container.Router(ctx)

	a.gRPCServer = grpcapp.New(
		a.container.log,
		a.cfg.Port,
	)
}

func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("сервер запущен", "port", a.cfg.GRPCServer.Port)

	go func() {
		a.gRPCServer.MustRun()
	}()

	//TODO Graceful shutdown
	<-ctx.Done()
	slog.Info("получен сигнал, завершаем...")

	stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), a.cfg.HTTPServer.ShutdownTimeout)
	defer shutdownCancel()

	if err := a.gRPCServer.Close(shutdownCtx); err != nil {
		slog.Error("ошибка при остановке сервера", "err", err)
	}

	slog.Info("сервер остановлен")

	closerCtx, closerCancel := context.WithTimeout(context.Background(), a.cfg.HTTPServer.CloseTimeout)
	defer closerCancel()

	if err := closer.CloseAll(closerCtx); err != nil {
		slog.Error("ошибки при закрытии ресурсов", "err", err)
	}

	return nil
}
