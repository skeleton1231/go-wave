package model

import (
	"time"
)

type User struct {
	Name            string     `gorm:"size:255;not null" json:"name"`
	Email           *string    `gorm:"size:255" json:"email"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt"`
	Password        string     `gorm:"size:255;not null" json:"-"`
	RememberToken   *string    `gorm:"size:100" json:"-"`
	StripeID        *string    `gorm:"size:255" json:"stripeId"`
	DiscordID       uint64     `gorm:"default:0" json:"discordId"`
	PMType          *string    `gorm:"size:255" json:"pmType"`
	PMLastFour      *string    `gorm:"size:4" json:"pmLastFour"`
	TrialEndsAt     *time.Time `json:"trialEndsAt"`
	TotalCredits    int        `gorm:"default:0" json:"totalCredits"`
}

// TableName overrides the table name used by User to `users`.
func (User) TableName() string {
	return "users"
}
