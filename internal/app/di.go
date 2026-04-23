package app

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"voice-chat-api/internal/closer"
	"voice-chat-api/internal/config"
	handler "voice-chat-api/internal/handlers"
	mw "voice-chat-api/internal/middlewares"
	repo "voice-chat-api/internal/repositories"
	service "voice-chat-api/internal/services"
	sessionstorage "voice-chat-api/internal/session_storage"
	"voice-chat-api/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type diContainer struct {
	m              sync.Mutex
	cfg            *config.Config
	secrets        *config.Secrets
	log            *slog.Logger
	storage        *storage.Storage
	transactor     *storage.Transactor
	sessionStorage *sessionstorage.SessionStorage
	userRepo       *repo.UserRepo
	tokenRepo      *repo.TokenRepo
	authService    *service.AuthService
	sessionService *service.SessionService
	userHandler    *handler.UserHandler
	tokenHandler   *handler.TokenHandler
	wsHandler      *handler.WSHandler
	router         *chi.Mux
}

func newDIContainer(cfg *config.Config, secrets *config.Secrets, log *slog.Logger) *diContainer {
	return &diContainer{
		cfg:     cfg,
		secrets: secrets,
		log:     log,
	}
}

func (c *diContainer) Storage(ctx context.Context) *storage.Storage {
	c.m.Lock()
	defer c.m.Unlock()
	if c.storage == nil {
		stg, err := storage.NewStorage(ctx, c.secrets, c.cfg)
		if err != nil {
			c.log.Error("failed to initialize storage", "err", err)
			os.Exit(1)
		}

		closer.Add("storage", func(_ context.Context) error {
			return stg.Close()
		})

		c.storage = stg
	}

	return c.storage
}

func (c *diContainer) Transactor(ctx context.Context) *storage.Transactor {
	c.m.Lock()
	defer c.m.Unlock()
	if c.transactor == nil {
		c.transactor = storage.NewTransactor(c.Storage(ctx))
	}

	return c.transactor
}

func (c *diContainer) SessionStorage(ctx context.Context) *sessionstorage.SessionStorage {
	c.m.Lock()
	defer c.m.Unlock()
	if c.sessionStorage == nil {
		stg := sessionstorage.NewSessionStorage()

		// closer.Add("storage", func(_ context.Context) error {
		// 	return stg.Close()
		// })

		c.sessionStorage = stg
	}

	return c.sessionStorage
}

func (c *diContainer) UserRepo(ctx context.Context) *repo.UserRepo {
	c.m.Lock()
	defer c.m.Unlock()
	if c.userRepo == nil {
		c.userRepo = repo.NewUserRepo(c.Transactor(ctx))
	}

	return c.userRepo
}

func (c *diContainer) TokenRepo(ctx context.Context) *repo.TokenRepo {
	c.m.Lock()
	defer c.m.Unlock()
	if c.tokenRepo == nil {
		c.tokenRepo = repo.NewTokenRepo(c.Transactor(ctx))
	}

	return c.tokenRepo
}

func (c *diContainer) AuthService(ctx context.Context) *service.AuthService {
	c.m.Lock()
	defer c.m.Unlock()
	if c.authService == nil {
		c.authService = service.NewAuthService(
			c.log,
			c.UserRepo(ctx),
			c.TokenRepo(ctx),
			c.Transactor(ctx),
			c.cfg.JWT.AccessTTL,
			c.cfg.JWT.RefreshTTL,
			c.secrets.JWTSecret,
		)
	}

	return c.authService
}

func (c *diContainer) SessionService(ctx context.Context) *service.SessionService {
	c.m.Lock()
	defer c.m.Unlock()
	if c.sessionService == nil {
		c.sessionService = service.NewSessionService(
			c.log,
			c.SessionStorage(ctx),
			// c.UserRepo(ctx),
			// c.TokenRepo(ctx),
			// c.Transactor(ctx),
			// c.cfg.JWT.AccessTTL,
			// c.cfg.JWT.RefreshTTL,
			// c.secrets.JWTSecret,
		)
	}

	return c.sessionService
}

func (c *diContainer) UserHandler(ctx context.Context) *handler.UserHandler {
	c.m.Lock()
	defer c.m.Unlock()
	if c.userHandler == nil {
		c.userHandler = handler.NewUserHandler(c.log, c.AuthService(ctx))
	}

	return c.userHandler
}

func (c *diContainer) TokenHandler(ctx context.Context) *handler.TokenHandler {
	c.m.Lock()
	defer c.m.Unlock()
	if c.tokenHandler == nil {
		c.tokenHandler = handler.NewTokenHandler(c.log, c.AuthService(ctx))
	}

	return c.tokenHandler
}

func (c *diContainer) WSHandler(ctx context.Context) *handler.WSHandler {
	c.m.Lock()
	defer c.m.Unlock()
	if c.wsHandler == nil {
		c.wsHandler = handler.NewWSHandler(c.log, c.SessionService(ctx))
	}

	return c.wsHandler
}

func (c *diContainer) Router(ctx context.Context) *chi.Mux {
	c.m.Lock()
	defer c.m.Unlock()
	if c.router == nil {
		c.router = chi.NewRouter()

		c.router.Use(middleware.RequestID)
		c.router.Use(middleware.RealIP)
		c.router.Use(middleware.Recoverer)
		c.router.Use(middleware.URLFormat)

		c.router.Route("/api", func(r chi.Router) {
			r.Route("/user", func(r chi.Router) {
				r.Post("/register", c.UserHandler(ctx).Register)
				r.Post("/login", c.UserHandler(ctx).Login)
			})
			r.Route("/token", func(r chi.Router) {
				r.Use(mw.AuthMiddleware(c.secrets.JWTSecret))
				r.Post("/refresh", c.TokenHandler(ctx).Refresh)
			})
			r.Route("/ws", func(r chi.Router) {
				r.Use(mw.AuthMiddleware(c.secrets.JWTSecret))
				r.Post("/", c.WSHandler(ctx).Handle)
			})
		})
	}

	return c.router
}

func (c *diContainer) Config() *config.Config {
	return c.cfg
}

func (c *diContainer) Secrets() *config.Secrets {
	return c.secrets
}

func (c *diContainer) Logger() *slog.Logger {
	return c.log
}
