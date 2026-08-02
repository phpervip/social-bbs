package repository

import "gorm.io/gorm"

// Migrate creates/updates the video_db schema (infra/mysql/init/03-video.sql).
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Video{}, &Upload{}, &TranscodeTask{})
}