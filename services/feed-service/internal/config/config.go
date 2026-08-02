// Package config loads Feed Service runtime configuration from environment variables.
package config

import "os"

// Config holds all runtime configuration for the Feed Service.
type Config struct {
	GRPCAddr      string
	DBDsn         string
	RedisAddr     string
	RedisPassword string
	KafkaAddr     string
	UserAddr      string
}

// Load reads configuration from environment variables, applying the P1 defaults
// (see plan §3.4) plus the P2 Kafka/User Service endpoints (plan T4 §6).
func Load() Config {
	return Config{
		GRPCAddr:      getEnv("FEED_GRPC_ADDR", ":9000"),
		DBDsn:         getEnv("FEED_DB_DSN", "feed:feed123@tcp(127.0.0.1:3306)/feed_db?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:     getEnv("FEED_REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("FEED_REDIS_PASSWORD", ""),
		KafkaAddr:     getEnv("FEED_KAFKA_ADDR", "localhost:9092"),
		UserAddr:      getEnv("FEED_USER_ADDR", "localhost:9001"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
