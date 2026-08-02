package kafka

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

// Client wraps the segmentio/kafka-go producer/consumer handles used by the
// Video Service (pure Go, no cgo).
type Client struct {
	addr   string
	writer *kafka.Writer
}

// NewClient returns a Client connected to the given broker address. The writer
// is created lazily on first publish; readers are created per consumer group.
func NewClient(addr string) *Client {
	return &Client{addr: addr}
}

// Publish sends one message to topic with key/value; the topic may be set
// per-message so a single writer serves every event.
func (c *Client) Publish(ctx context.Context, topic string, key, value []byte) error {
	if c.writer == nil {
		c.writer = kafka.NewWriter(kafka.WriterConfig{
			Brokers:      []string{c.addr},
			Balancer:     &kafka.Hash{},
			RequiredAcks: int(kafka.RequireAll),
		})
	}
	return c.writer.WriteMessages(ctx, kafka.Message{Topic: topic, Key: key, Value: value})
}

// NewTranscodeTaskReader returns a reader in the video-transcode group
// consuming video:transcode-task (FFmpeg worker).
func (c *Client) NewTranscodeTaskReader() *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{c.addr},
		GroupID:  GroupTranscode,
		Topic:    TopicTranscodeTask,
		MinBytes: 10e3,
		MaxBytes: 10e6,
		MaxWait:  500 * time.Millisecond,
	})
}

// Close releases the writer (and any in-flight publishes).
func (c *Client) Close() error {
	if c.writer != nil {
		return c.writer.Close()
	}
	return nil
}