package config

import (
	"errors"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

var ErrConfigPathNotSet = errors.New("CONFIG_PATH is not set")

type Config struct {
	Env        string     `yaml:"env" env:"ENV" env-default:"development"`
	GRPCServer GRPCServer `yaml:"grpc_server"`
	Signaling  Signaling  `yaml:"signaling"`
	WebRTC     WebRTC     `yaml:"webrtc"`
}

type GRPCServer struct {
	Port            int                 `yaml:"port" env-default:"8085"`
	ShutdownTimeout time.Duration       `yaml:"shutdown_timeout" env-default:"15s"`
	Keepalive       GRPCServerKeepalive `yaml:"keepalive"`
}

type GRPCServerKeepalive struct {
	Time                time.Duration `yaml:"time" env-default:"20s"`
	Timeout             time.Duration `yaml:"timeout" env-default:"10s"`
	MinTime             time.Duration `yaml:"min_time" env-default:"10s"`
	PermitWithoutStream bool          `yaml:"permit_without_stream" env-default:"true"`
}

type Signaling struct {
	ReattachGracePeriod time.Duration `yaml:"reattach_grace_period" env-default:"30s"`
	EmptyRoomTTL        time.Duration `yaml:"empty_room_ttl" env-default:"5m"`
}

type WebRTC struct {
	DefaultTURNUsername   string      `env:"TURN_USER" env-default:"voicechat"`
	DefaultTURNCredential string      `env:"TURN_PASSWORD" env-default:"voicechatpass"`
	ICEServers            []ICEServer `yaml:"ice_servers"`
}

type ICEServer struct {
	URLs       []string `yaml:"urls"`
	Username   string   `yaml:"username"`
	Credential string   `yaml:"credential"`
}

func (c *Config) ServerShutdownTimeout() time.Duration {
	if c.GRPCServer.ShutdownTimeout > 0 {
		return c.GRPCServer.ShutdownTimeout
	}

	return 15 * time.Second
}

func Load() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		return nil, ErrConfigPathNotSet
	}

	if _, err := os.Stat(configPath); err != nil {
		return nil, err
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
