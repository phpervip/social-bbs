// Command server runs the Feed Service gRPC server (P2).
//
// Wiring order: config → MySQL (GORM, AutoMigrate + seed) → Redis (go-redis) →
// repositories → User Service client → Kafka client → outbox dispatcher +
// compensation + Kafka consumers (fanout / follow-changed) → gRPC server (+
// standard health service) → graceful shutdown (SIGINT/SIGTERM).
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

	"social-bbs/feed-service/internal/config"
	"social-bbs/feed-service/internal/handler"
	"social-bbs/feed-service/internal/kafka"
	"social-bbs/feed-service/internal/repository"
	"social-bbs/feed-service/internal/service"
	"social-bbs/feed-service/internal/worker"
	feedpb "social-bbs/feed-service/proto/gen"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("feed-service exited with error", "error", err)
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
	if err := repository.SeedUsersIfEmpty(db); err != nil {
		return err
	}
	logger.Info("database ready (migrated + seeded)")

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancel()
		return err
	}
	cancel()
	logger.Info("redis ready", "addr", cfg.RedisAddr)

	cache := repository.NewRedisCache(rdb)
	postRepo := repository.NewPostRepo(db)
	outboxRepo := repository.NewOutboxRepo(db)
	likeRepo := repository.NewLikeRepo(db)
	commentRepo := repository.NewCommentRepo(db)

	userClient := repository.NewUserClient(cfg.UserAddr, cache)
	fanout := worker.NewFanoutHandler(cache, userClient)
	kafkaClient := kafka.NewClient(cfg.KafkaAddr)
	logger.Info("kafka client ready", "addr", cfg.KafkaAddr)

	postSvc := service.NewPostService(postRepo, outboxRepo, userClient, likeRepo, cache)
	timelineSvc := service.NewTimelineService(postRepo, likeRepo, cache, userClient)
	interactionSvc := service.NewInteractionService(likeRepo, cache)
	commentSvc := service.NewCommentService(commentRepo, cache)
	searchSvc := service.NewSearchService(postRepo, likeRepo, userClient)

	// P2 workers: outbox dispatcher (resident poll) + compensation (5s ticker)
	// + the two Kafka consumers (design §5.2/§5.3/§5.4).
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	dispatcher := worker.NewDispatcher(outboxRepo, kafkaClient, logger)
	consumer := worker.NewConsumer(fanout, postRepo, cache, logger)

	wg.Add(4)
	go func() { defer wg.Done(); dispatcher.Run(workerCtx) }()
	go func() { defer wg.Done(); dispatcher.RunCompensation(workerCtx) }()
	go func() { defer wg.Done(); _ = consumer.RunFanout(workerCtx, kafkaClient.NewFanoutReader()) }()
	go func() { defer wg.Done(); _ = consumer.RunTimeline(workerCtx, kafkaClient.NewTimelineReader()) }()
	logger.Info("kafka workers started (outbox dispatcher, compensation, feed-fanout, feed-timeline)")

	grpcSrv := grpc.NewServer()
	feedpb.RegisterFeedServiceServer(grpcSrv, handler.NewFeedHandler(postSvc, timelineSvc, interactionSvc, commentSvc, searchSvc))

	// Standard gRPC health service (brief §4).
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus("feed.v1.FeedService", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		stopWorkers()
		return err
	}

	// Graceful shutdown: stop accepting RPCs (draining in-flight handlers so no
	// new outbox rows are written), then cancel the Kafka workers, wait for them
	// to drain, and close the external clients.
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

	logger.Info("feed-service listening", "addr", lis.Addr().String())
	if err := grpcSrv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		stopWorkers()
		return err
	}
	logger.Info("feed-service stopped cleanly")
	return nil
}
