// Command server runs the Video Service gRPC server (P3).
//
// Wiring order: config → MySQL (GORM, AutoMigrate) → Redis (go-redis) → S3
// (MinIO, EnsureBucket) → repositories → User Service client → Kafka client →
// FFmpeg worker (Kafka consumer) → gRPC server (+ standard health service) →
// graceful shutdown (SIGINT/SIGTERM).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"social-bbs/video-service/internal/config"
	"social-bbs/video-service/internal/handler"
	"social-bbs/video-service/internal/kafka"
	"social-bbs/video-service/internal/repository"
	"social-bbs/video-service/internal/service"
	"social-bbs/video-service/internal/worker"
	videopb "social-bbs/video-service/proto/gen"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("video-service exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := config.Load()

	db, err := gorm.Open(mysql.Open(cfg.DBDsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return err
	}
	if err := repository.Migrate(db); err != nil {
		return err
	}
	logger.Info("database ready (migrated)")

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancel()
		return err
	}
	cancel()
	logger.Info("redis ready", "addr", cfg.RedisAddr)

	s3, err := repository.NewS3Client(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Region)
	if err != nil {
		return err
	}
	bucketCtx, bucketCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := s3.EnsureBucket(bucketCtx, cfg.S3Bucket); err != nil {
		bucketCancel()
		return err
	}
	bucketCancel()
	logger.Info("s3 ready", "endpoint", cfg.S3Endpoint, "bucket", cfg.S3Bucket)

	cacheRepo := repository.NewRedisCache(rdb)
	videoRepo := repository.NewVideoRepo(db)
	uploadRepo := repository.NewUploadRepo(db)
	transcodeRepo := repository.NewTranscodeRepo(db)
	userClient := repository.NewUserClient(cfg.UserAddr)

	kafkaClient := kafka.NewClient(cfg.KafkaAddr)
	logger.Info("kafka client ready", "addr", cfg.KafkaAddr)

	uploadSvc := service.NewUploadService(videoRepo, uploadRepo, transcodeRepo, s3, cacheRepo, kafkaClient, cfg.S3Bucket)
	playbackSvc := service.NewPlaybackService(videoRepo, transcodeRepo, s3, cacheRepo, userClient, cfg.S3Bucket)
	transcodeSvc := service.NewTranscodeService(videoRepo, transcodeRepo, kafkaClient)

	// FFmpeg worker: consumes video:transcode-task and transcodes to HLS.
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ffmpegWorker := worker.NewFFmpegWorker(videoRepo, transcodeRepo, s3, transcodeSvc, cacheRepo, logger, cfg.S3Bucket, os.Getenv("VIDEO_FFMPEG_PATH"))
	wg.Add(1)
	go func() { defer wg.Done(); _ = ffmpegWorker.Run(workerCtx, kafkaClient.NewTranscodeTaskReader()) }()
	logger.Info("ffmpeg worker started (video:transcode-task)")

	grpcSrv := grpc.NewServer()
	videopb.RegisterVideoServiceServer(grpcSrv, handler.NewVideoHandler(uploadSvc, playbackSvc))

	// Standard gRPC health service.
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("video.v1.VideoService", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		stopWorkers()
		return err
	}

	// Graceful shutdown: stop accepting RPCs, cancel the Kafka worker, wait for
	// it to drain, then close the external clients.
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-stopCh
		logger.Info("shutdown signal received", "signal", sig.String())
		healthSrv.Shutdown()
		grpcSrv.GracefulStop()
		stopWorkers()
		wg.Wait()
		_ = kafkaClient.Close()
		if err := userClient.Close(); err != nil {
			logger.Warn("user client close failed", "error", err)
		}
		_ = rdb.Close()
	}()

	logger.Info("video-service listening", "addr", lis.Addr().String())
	if err := grpcSrv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		stopWorkers()
		return err
	}
	logger.Info("video-service stopped cleanly")
	return nil
}