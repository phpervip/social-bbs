package repository

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Client wraps the MinIO-compatible S3 client (aws-sdk-go-v2) used for
// multipart uploads, HLS/thumbnail storage and presigned playback URLs.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client builds an S3 client pointed at the MinIO endpoint using
// path-style addressing (MinIO does not support virtual-host buckets).
func NewS3Client(endpoint, accessKey, secretKey, region string) (*S3Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://" + endpoint)
		o.UsePathStyle = true
	})
	return &S3Client{client: client}, nil
}

// EnsureBucket creates the configured bucket if it does not exist (idempotent).
func (c *S3Client) EnsureBucket(ctx context.Context, bucket string) error {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		_, err = c.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
		return err
	}
	// HeadBucket may return a generic error for a missing bucket; attempt create.
	_, createErr := c.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if createErr != nil {
		return createErr
	}
	return nil
}

// CreateMultipartUpload starts a multipart upload and returns its upload ID.
func (c *S3Client) CreateMultipartUpload(ctx context.Context, bucket, key, contentType string) (string, error) {
	out, err := c.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.UploadId), nil
}

// UploadPart uploads one part and returns its ETag.
func (c *S3Client) UploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int32, data []byte) (string, error) {
	out, err := c.client.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
		Body:       bytesReader(data),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.ETag), nil
}

// CompleteMultipartUpload finalizes the multipart upload (S3 merges the parts).
func (c *S3Client) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []types.CompletedPart) error {
	_, err := c.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	})
	return err
}

// DownloadObject streams an object to w (used by the FFmpeg worker to fetch the
// raw video into a temp file).
func (c *S3Client) DownloadObject(ctx context.Context, bucket, key string, w io.Writer) error {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	_, err = io.Copy(w, out.Body)
	return err
}

// UploadObject writes a single object (HLS .m3u8 / .ts segment / thumbnail).
func (c *S3Client) UploadObject(ctx context.Context, bucket, key, contentType string, body io.Reader) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	return err
}

// Presign returns a time-limited GET URL for key.
func (c *S3Client) Presign(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	ps := s3.NewPresignClient(c.client)
	req, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func bytesReader(b []byte) io.ReadSeeker {
	return &sliceReader{b: b}
}

type sliceReader struct {
	b   []byte
	off int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}

func (r *sliceReader) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = int64(r.off)
	case io.SeekEnd:
		base = int64(len(r.b))
	}
	pos := base + offset
	if pos < 0 || pos > int64(len(r.b)) {
		return 0, errors.New("s3: invalid seek position")
	}
	r.off = int(pos)
	return pos, nil
}