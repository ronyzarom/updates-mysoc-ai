package config

import (
	"os"
	"strconv"
)

// Config holds all configuration for the update server
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Storage  StorageConfig
	Auth     AuthConfig
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret string
	Issuer    string
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port     int
	Host     string
	APIKey   string // Admin API key for management endpoints
	CORSOrigins []string
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

// StorageConfig holds artifact storage configuration
type StorageConfig struct {
	Type     string // "local" or "s3"
	LocalPath string
	// S3 configuration (for future use)
	S3Bucket   string
	S3Region   string
	S3Endpoint string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:     getEnvInt("SERVER_PORT", 8080),
			Host:     getEnv("SERVER_HOST", "0.0.0.0"),
			APIKey:   getEnv("ADMIN_API_KEY", ""),
			CORSOrigins: []string{"*"},
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			Name:     getEnv("DB_NAME", "mysoc_updates"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", ""),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
		},
		Storage: StorageConfig{
			Type:      getEnv("STORAGE_TYPE", "local"),
			LocalPath: getEnv("STORAGE_LOCAL_PATH", "./artifacts"),
			S3Bucket:  getEnv("STORAGE_S3_BUCKET", ""),
			S3Region:  getEnv("STORAGE_S3_REGION", ""),
			S3Endpoint: getEnv("STORAGE_S3_ENDPOINT", ""),
		},
		Auth: AuthConfig{
			JWTSecret: getEnv("JWT_SECRET", "change-this-secret-in-production"),
			Issuer:    getEnv("JWT_ISSUER", "updates.mysoc.ai"),
		},
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

