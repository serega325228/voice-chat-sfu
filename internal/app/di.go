package app

import (
	"log/slog"
	"sync"
	grpcapp "voice-chat-sfu/internal/app/grpc"
	"voice-chat-sfu/internal/config"
	"voice-chat-sfu/internal/grpc/session"
	"voice-chat-sfu/internal/services"
	"voice-chat-sfu/internal/storage"
)

type diContainer struct {
	cfg *config.Config
	log *slog.Logger

	storageOnce sync.Once
	storage     *storage.SFUStorage

	sessionServiceOnce sync.Once
	sessionService     *service.SFUService
	sessionServiceErr  error

	grpcAppOnce sync.Once
	grpcApp     *grpcapp.App
}

func newDIContainer(cfg *config.Config, log *slog.Logger) *diContainer {
	return &diContainer{
		cfg: cfg,
		log: log,
	}
}

func (c *diContainer) Storage() *storage.SFUStorage {
	c.storageOnce.Do(func() {
		c.storage = storage.NewSFUStorage()
	})

	return c.storage
}

func (c *diContainer) SessionService() (*service.SFUService, error) {
	c.sessionServiceOnce.Do(func() {
		c.sessionService, c.sessionServiceErr = service.NewSFUService(c.log, c.Storage())
	})

	return c.sessionService, c.sessionServiceErr
}

func (c *diContainer) GRPCApp() (*grpcapp.App, error) {
	c.grpcAppOnce.Do(func() {
		sessionService, err := c.SessionService()
		if err != nil {
			c.sessionServiceErr = err
			return
		}

		c.grpcApp = grpcapp.New(c.log, c.cfg.GRPCServer.Port, sessionService)
	})

	if c.sessionServiceErr != nil {
		return nil, c.sessionServiceErr
	}

	return c.grpcApp, nil
}
