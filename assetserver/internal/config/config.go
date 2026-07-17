// Package config — 配置加载 (环境变量 + YAML)
// 对应架构文档 §2.3 新增基础设施依赖
package config

import (
	"os"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Vault    VaultConfig
	Auth     AuthConfig
}

type ServerConfig struct {
	Host            string        `default:"0.0.0.0"`
	Port            string        `default:"8080"`
	ReadTimeout     time.Duration `default:"30s"`
	WriteTimeout    time.Duration `default:"30s"`
	ShutdownTimeout time.Duration `default:"10s"`
}

type DatabaseConfig struct {
	Host            string `default:"localhost"`
	Port            string `default:"5432"`
	Name            string `default:"assetdb"`
	User            string `default:"app_writer"`
	Password        string // 从环境变量 DATABASE_PASSWORD 读取
	Schema          string `default:"assets"`
	MaxConns        int    `default:"25"`
	MinConns        int    `default:"5"`
	MaxConnLifetime time.Duration `default:"1h"`
	MaxConnIdleTime time.Duration `default:"10m"`
	AutoMigrate     bool   `default:"false"` // 开发环境 true, 生产 false
}

type RedisConfig struct {
	Addrs    []string `default:"localhost:6379"` // Sentinel 地址列表
	MasterName string `default:"mymaster"`        // Sentinel master name
	Password string   // 从环境变量 REDIS_PASSWORD 读取
	DB       int      `default:"0"`
	PoolSize int      `default:"10"`
}

type VaultConfig struct {
	Addr              string        `default:"http://localhost:8200"`
	Token             string        // 从环境变量 VAULT_TOKEN 读取
	JWTKeyPath        string        `default:"transit/keys/jwt-ed25519"`
	WebhookKeyPath    string        `default:"transit/keys/webhook-aes"`
	HealthCheckInterval time.Duration `default:"30s"`
	RetryMaxAttempts  int           `default:"10"`
	RetryBaseDelay    time.Duration `default:"2s"`
	RetryMaxDelay     time.Duration `default:"30s"`
}

type AuthConfig struct {
	AccessTokenTTL  time.Duration `default:"15m"`
	RefreshTokenTTL time.Duration `default:"7d"`
	Issuer          string        `default:"asset-db-api"`
	Audience        string        `default:"asset-db"`
	FailOpenGet     bool          `default:"true"`  // Redis 故障时读操作 fail-open
	FailOpenAgentSync bool        `default:"true"`  // Agent 上报 fail-open
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:            getEnv("SERVER_HOST", "0.0.0.0"),
			Port:            getEnv("SERVER_PORT", "8080"),
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			Name:     getEnv("DB_NAME", "assetdb"),
			User:     getEnv("DB_USER", "app_writer"),
			Password: os.Getenv("DATABASE_PASSWORD"),
			Schema:   "assets",
			MaxConns: 25,
			MinConns: 5,
			MaxConnLifetime: 1 * time.Hour,
			MaxConnIdleTime: 10 * time.Minute,
		},
		Redis: RedisConfig{
			Addrs:      []string{getEnv("REDIS_ADDR", "localhost:6379")},
			MasterName: getEnv("REDIS_MASTER", "mymaster"),
			Password:   os.Getenv("REDIS_PASSWORD"),
			DB:         0,
			PoolSize:   10,
		},
		Vault: VaultConfig{
			Addr:                getEnv("VAULT_ADDR", "http://localhost:8200"),
			Token:               os.Getenv("VAULT_TOKEN"),
			JWTKeyPath:          "transit/keys/jwt-ed25519",
			WebhookKeyPath:      "transit/keys/webhook-aes",
			HealthCheckInterval: 30 * time.Second,
			RetryMaxAttempts:    10,
			RetryBaseDelay:      2 * time.Second,
			RetryMaxDelay:       30 * time.Second,
		},
		Auth: AuthConfig{
			AccessTokenTTL:   15 * time.Minute,
			RefreshTokenTTL:  7 * 24 * time.Hour,
			Issuer:            "asset-db-api",
			Audience:          "asset-db",
		},
	}
	return cfg, nil
}

func (d *DatabaseConfig) DSN() string {
	sslMode := os.Getenv("DB_SSLMODE")
	if sslMode == "" {
		sslMode = "require"
	}
	return "postgres://" + d.User + ":" + d.Password +
		"@" + d.Host + ":" + d.Port + "/" + d.Name +
		"?sslmode=" + sslMode + "&search_path=" + d.Schema
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
