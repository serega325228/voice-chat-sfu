package app

import (
	"log/slog"
	"sync"
	grpcapp "voice-chat-sfu/internal/app/grpc"
	"voice-chat-sfu/internal/config"
	"voice-chat-sfu/internal/grpc/session"
	"voice-chat-sfu/internal/services"
	"voice-chat-sfu/internal/storage"

	"github.com/pion/webrtc/v4"
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
		c.sessionService, c.sessionServiceErr = service.NewSFUService(c.log, c.Storage(), service.Config{
			ICEServers: c.webRTCIceServers(),
		})
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

		c.grpcApp = grpcapp.New(c.log, grpcapp.Config{
			Port:                c.cfg.GRPCServer.Port,
			KeepaliveTime:       c.cfg.GRPCServer.Keepalive.Time,
			KeepaliveTimeout:    c.cfg.GRPCServer.Keepalive.Timeout,
			KeepaliveMinTime:    c.cfg.GRPCServer.Keepalive.MinTime,
			PermitWithoutStream: c.cfg.GRPCServer.Keepalive.PermitWithoutStream,
		}, session.Config{
			StreamReattachGracePeriod: c.cfg.Signaling.ReattachGracePeriod,
		}, sessionService)
	})

	if c.sessionServiceErr != nil {
		return nil, c.sessionServiceErr
	}

	return c.grpcApp, nil
}

func (c *diContainer) webRTCIceServers() []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(c.cfg.WebRTC.ICEServers))

	for _, server := range c.cfg.WebRTC.ICEServers {
		if len(server.URLs) == 0 {
			continue
		}

		servers = append(servers, webrtc.ICEServer{
			URLs:       append([]string(nil), server.URLs...),
			Username:   firstNonEmpty(c.cfg.WebRTC.DefaultTURNUsername, server.Username),
			Credential: firstNonEmpty(c.cfg.WebRTC.DefaultTURNCredential, server.Credential),
		})
	}

	return servers
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
