// Package config loads Video Service runtime configuration from environment variables.
package config

import "os"

// Config holds all runtime configuration for the Video Service.
type Config struct {
	GRPCAddr      string
	DBDsn         string
	RedisAddr     string
	RedisPassword string
	KafkaAddr     string
	S3Endpoint    string
	S3AccessKey   string
	S3SecretKey   string
	S3Bucket      string
	S3Region      string
	UserAddr      string
}

// Load reads configuration from environment variables (prefix VIDEO_), applying
// the P3 defaults (see infra/mysql/init/03-video.sql and infra/README.md).
func Load() Config {
	return Config{
		GRPCAddr:      getEnv("VIDEO_GRPC_ADDR", ":9002"),
		DBDsn:         getEnv("VIDEO_DB_DSN", "video:video123@tcp(127.0.0.1:3306)/video_db?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:     getEnv("VIDEO_REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("VIDEO_REDIS_PASSWORD", ""),
		KafkaAddr:     getEnv("VIDEO_KAFKA_ADDR", "localhost:9092"),
		S3Endpoint:    getEnv("VIDEO_S3_ENDPOINT", "localhost:9000"),
		S3AccessKey:   getEnv("VIDEO_S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:   getEnv("VIDEO_S3_SECRET_KEY", "minioadmin"),
		S3Bucket:      getEnv("VIDEO_S3_BUCKET", "videos"),
		S3Region:      getEnv("VIDEO_S3_REGION", "us-east-1"),
		UserAddr:      getEnv("VIDEO_USER_ADDR", "localhost:9001"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}