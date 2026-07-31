package repository

import "gorm.io/gorm"

// Migrate creates/updates the feed_db schema (plan §3.1).
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &PostRow{}, &PostLike{}, &PostComment{}, &OutboxEvent{})
}

type seedUser struct {
	Username    string
	DisplayName string
}

// SeedUsers matches the gateway's hardcoded dev user list exactly (plan §7).
var SeedUsers = []seedUser{
	{Username: "bob", DisplayName: "Bob咖啡师"},
	{Username: "alice", DisplayName: "Alice设计师"},
	{Username: "carol", DisplayName: "Carol摄影师"},
	{Username: "dave", DisplayName: "Dave开发者"},
}

// SeedUsers writes the 4 P1 demo users only when the users table is empty.
func SeedUsersIfEmpty(db *gorm.DB) error {
	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, s := range SeedUsers {
		if err := db.Create(&User{Username: s.Username, DisplayName: s.DisplayName}).Error; err != nil {
			return err
		}
	}
	return nil
}
