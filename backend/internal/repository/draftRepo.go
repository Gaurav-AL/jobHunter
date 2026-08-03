package repository

import "gorm.io/datatypes"

type DraftStatus string

const (
    DraftStatusDraft  DraftStatus = "DRAFT"
    DraftStatusSent   DraftStatus = "SENT"
    DraftStatusFailed DraftStatus = "FAILED"
)

type Draft struct {
	ID uint `gorm:"primaryKey"`
	CompanyName string
	RecipientEmail string
	Body string
	Status DraftStatus
	CreatedAt datatypes.Date
	UpdatedAt datatypes.Date
}