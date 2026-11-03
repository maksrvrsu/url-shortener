package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type Config struct {
	Env         string         `yaml:"env" env:"ENV" env-default:"local"`
	StoragePath string         `yaml:"storage_path" env:"DATABASE_URL"`
	HTTPServer  HTTPServer     `yaml:"http_server"`
	Postgres    PostgresConfig `yaml:"postgres"`
}

type HTTPServer struct {
	Address     string        `yaml:"address" env:"HTTP_SERVER_ADDRESS" env-default:"0.0.0.0:8082"`
	Timeout     time.Duration `yaml:"timeout" env:"HTTP_SERVER_TIMEOUT" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env:"HTTP_SERVER_IDLE_TIMEOUT" env-default:"60s"`
	User        string        `yaml:"user" env:"HTTP_SERVER_USER" env-required:"true"`
	Password    string        `yaml:"password" env:"HTTP_SERVER_PASSWORD" env-required:"true"`
}

type PostgresConfig struct {
	User     string `yaml:"user" env:"POSTGRES_USER"`
	Password string `yaml:"password" env:"POSTGRES_PASSWORD"`
	Host     string `yaml:"host" env:"POSTGRES_HOST" env-default:"localhost"`
	Port     string `yaml:"port" env:"POSTGRES_PORT" env-default:"5432"`
	DBName   string `yaml:"db_name" env:"POSTGRES_DB" env-default:"url_shortener"`
	SSLMode  string `yaml:"sslmode" env:"POSTGRES_SSLMODE" env-default:"disable"`
}

func MustLoad() *Config {
	// Try loading .env file if present (ignored if file does not exist)
	_ = godotenv.Load()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/local.yaml"
	}

	return MustLoadByPath(configPath)
}

func MustLoadByPath(configPath string) *Config {
	var cfg Config

	if _, err := os.Stat(configPath); err == nil {
		if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
			log.Fatalf("cannot read config from %s: %s", configPath, err)
		}
	} else {
		// Fallback to pure environment variables if YAML file is absent
		if err := cleanenv.ReadEnv(&cfg); err != nil {
			log.Fatalf("cannot read config from environment: %s", err)
		}
	}

	// If StoragePath (DATABASE_URL) is not explicitly provided, build from individual Postgres vars
	if cfg.StoragePath == "" && cfg.Postgres.User != "" {
		cfg.StoragePath = fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.Postgres.User,
			cfg.Postgres.Password,
			cfg.Postgres.Host,
			cfg.Postgres.Port,
			cfg.Postgres.DBName,
			cfg.Postgres.SSLMode,
		)
	}

	if cfg.StoragePath == "" {
		log.Fatal("storage_path (DATABASE_URL or POSTGRES_* credentials) is required")
	}

	return &cfg
}
