// Package worker implements the P3 FFmpeg transcode worker: it consumes
// video:transcode-task, downloads the raw video from S3, transcodes one quality
// to HLS, uploads the segments + thumbnail, and reports back to the
// TranscodeService.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	kafkago "github.com/segmentio/kafka-go"

	"social-bbs/video-service/internal/kafka"
	"social-bbs/video-service/internal/repository"
	"social-bbs/video-service/internal/service"
)

// FFmpegWorker consumes transcode tasks and runs FFmpeg.
type FFmpegWorker struct {
	videos     repository.VideoRepo
	tasks      repository.TranscodeRepo
	s3         *repository.S3Client
	transcode  service.TranscodeService
	cache      repository.Cache
	logger     *slog.Logger
	bucket     string
	ffmpegPath string
}

// NewFFmpegWorker wires the worker.
func NewFFmpegWorker(videos repository.VideoRepo, tasks repository.TranscodeRepo, s3 *repository.S3Client, transcode service.TranscodeService, cache repository.Cache, logger *slog.Logger, bucket, ffmpegPath string) *FFmpegWorker {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return &FFmpegWorker{videos: videos, tasks: tasks, s3: s3, transcode: transcode, cache: cache, logger: logger, bucket: bucket, ffmpegPath: ffmpegPath}
}

// Run consumes video:transcode-task until ctx is cancelled.
func (w *FFmpegWorker) Run(ctx context.Context, reader *kafkago.Reader) error {
	defer reader.Close()
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			w.logger.Error("kafka read failed", "error", err)
			continue
		}
		if err := w.handle(ctx, msg.Value); err != nil {
			w.logger.Error("transcode task failed", "error", err)
		}
	}
}

func (w *FFmpegWorker) handle(ctx context.Context, value []byte) error {
	var ev kafka.TranscodeTaskEvent
	if err := json.Unmarshal(value, &ev); err != nil {
		return err
	}
	if ev.TaskID == 0 || ev.VideoID == 0 || ev.Quality == "" {
		return nil
	}

	lockKey := repository.TranscodeLockKey(ev.TaskID)
	ok, err := w.cache.SetNX(ctx, lockKey, "1", repository.TranscodeLockTTL)
	if err != nil {
		return err
	}
	if !ok {
		return nil // another worker is processing this task
	}
	defer w.cache.Del(ctx, lockKey)

	if err := w.tasks.UpdateStatus(ctx, ev.TaskID, "processing", ""); err != nil {
		return err
	}

	video, err := w.videos.GetByID(ctx, ev.VideoID)
	if err != nil {
		return err
	}

	workDir, err := os.MkdirTemp("", "video-transcode-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	rawPath := filepath.Join(workDir, "input.mp4")
	if err := w.downloadRaw(ctx, video.RawKey, rawPath); err != nil {
		w.fail(ctx, ev, err)
		return err
	}

	if err := w.transcodeQuality(ctx, ev, rawPath, workDir); err != nil {
		w.fail(ctx, ev, err)
		return err
	}
	if err := w.uploadHLS(ctx, ev, workDir); err != nil {
		w.fail(ctx, ev, err)
		return err
	}
	if err := w.generateThumbnail(ctx, ev, rawPath, workDir); err != nil {
		w.fail(ctx, ev, err)
		return err
	}

	if err := w.transcode.CompleteTranscode(ctx, ev.VideoID, ev.Quality); err != nil {
		return err
	}
	w.logger.Info("transcode completed", "video_id", ev.VideoID, "quality", ev.Quality)
	return nil
}

func (w *FFmpegWorker) downloadRaw(ctx context.Context, rawKey, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	return w.s3.DownloadObject(ctx, w.bucket, rawKey, f)
}

// fail reports a failed transcode to the TranscodeService (retry/requeue logic).
func (w *FFmpegWorker) fail(ctx context.Context, ev kafka.TranscodeTaskEvent, cause error) {
	if err := w.transcode.FailTranscode(ctx, ev.VideoID, ev.Quality, cause.Error()); err != nil {
		w.logger.Error("fail transcode reporting failed", "video_id", ev.VideoID, "quality", ev.Quality, "error", err)
	}
}

// transcodeQuality runs one FFmpeg HLS pass for the requested quality.
func (w *FFmpegWorker) transcodeQuality(ctx context.Context, ev kafka.TranscodeTaskEvent, rawPath, workDir string) error {
	outDir := filepath.Join(workDir, ev.Quality)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	height := strings.TrimSuffix(ev.Quality, "p")
	args := []string{
		"-y", "-i", rawPath,
		"-vf", "scale=-2:" + height,
		"-c:v", "libx264", "-preset", "fast", "-crf", "23",
		"-c:a", "aac", "-b:a", "128k",
		"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_segment_filename", filepath.Join(outDir, "seg_%03d.ts"),
		filepath.Join(outDir, "index.m3u8"),
	}
	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// uploadHLS walks the quality output dir and uploads every file to
// hls/{video_id}/{quality}/{name}.
func (w *FFmpegWorker) uploadHLS(ctx context.Context, ev kafka.TranscodeTaskEvent, workDir string) error {
	outDir := filepath.Join(workDir, ev.Quality)
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(outDir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		contentType := "application/vnd.apple.mpegurl"
		if strings.HasSuffix(e.Name(), ".ts") {
			contentType = "video/mp2t"
		}
		key := fmt.Sprintf("hls/%d/%s/%s", ev.VideoID, ev.Quality, e.Name())
		uploadErr := w.s3.UploadObject(ctx, w.bucket, key, contentType, f)
		f.Close()
		if uploadErr != nil {
			return uploadErr
		}
	}
	return nil
}

// generateThumbnail extracts a frame at 1s and uploads it to thumb/{video_id}.jpg.
func (w *FFmpegWorker) generateThumbnail(ctx context.Context, ev kafka.TranscodeTaskEvent, rawPath, workDir string) error {
	thumbPath := filepath.Join(workDir, "thumb.jpg")
	args := []string{"-y", "-i", rawPath, "-ss", "00:00:01", "-vframes", "1", thumbPath}
	cmd := exec.CommandContext(ctx, w.ffmpegPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg thumb: %w: %s", err, strings.TrimSpace(string(out)))
	}
	f, err := os.Open(thumbPath)
	if err != nil {
		return err
	}
	defer f.Close()
	thumbKey := fmt.Sprintf("thumb/%d.jpg", ev.VideoID)
	if err := w.s3.UploadObject(ctx, w.bucket, thumbKey, "image/jpeg", f); err != nil {
		return err
	}
	video, err := w.videos.GetByID(ctx, ev.VideoID)
	if err != nil {
		return err
	}
	video.ThumbKey = thumbKey
	return w.videos.Update(ctx, video)
}