package repository

import "time"

// Video maps to the videos table (infra/mysql/init/03-video.sql).
type Video struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	UploaderID  uint64    `gorm:"column:uploader_id;index"`
	Title       string    `gorm:"size:255;default:''"`
	Description string    `gorm:"size:1000;default:''"`
	Visibility  string    `gorm:"size:20;default:'public'"` // public/followers_only/private
	Status      string    `gorm:"size:20;default:'pending';index"` // pending/processing/completed/failed
	RawKey      string    `gorm:"column:raw_key;size:500;default:''"`
	HLSKey      string    `gorm:"column:hls_key;size:500;default:''"`
	ThumbKey    string    `gorm:"column:thumb_key;size:500;default:''"`
	Duration    uint32    `gorm:"default:0"`
	CreatedAt   time.Time `gorm:"autoCreateTime(3)"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime(3)"`
}

// TableName pins the model to the frozen schema table "videos".
func (Video) TableName() string { return "videos" }

// Upload maps to the uploads table (multipart upload session tracking).
type Upload struct {
	UploadID       string    `gorm:"primaryKey;size:128"`
	VideoID        uint64    `gorm:"column:video_id;index"`
	Filename       string    `gorm:"size:255;default:''"`
	ContentType    string    `gorm:"column:content_type;size:128;default:'video/mp4'"`
	TotalSize      int64     `gorm:"column:total_size;default:0"`
	ChunkSize      uint32    `gorm:"column:chunk_size;default:5242880"`
	TotalChunks    uint32    `gorm:"column:total_chunks;default:0"`
	ReceivedChunks uint32    `gorm:"column:received_chunks;default:0"`
	Status         string    `gorm:"size:20;default:'pending'"` // pending/uploading/completed/aborted
	CreatedAt      time.Time `gorm:"autoCreateTime(3)"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime(3)"`
}

// TableName pins the GORM model to the frozen schema table "uploads".
func (Upload) TableName() string { return "uploads" }

// TranscodeTask maps to the transcode_tasks table (FFmpeg job records).
type TranscodeTask struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	VideoID    uint64    `gorm:"column:video_id;index"`
	Quality    string    `gorm:"size:16"` // 720p/480p/360p
	Status     string    `gorm:"size:20;default:'pending';index"` // pending/processing/completed/failed
	RetryCount uint32    `gorm:"column:retry_count;default:0"`
	MaxRetries uint32    `gorm:"column:max_retries;default:3"`
	ErrorMsg   string    `gorm:"column:error_msg;size:1000;default:''"`
	CreatedAt  time.Time `gorm:"autoCreateTime(3)"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime(3)"`
}

// TableName returns the GORM model to the frozen schema table "transcode_tasks".
func (TranscodeTask) TableName() string { return "transcode_tasks" }