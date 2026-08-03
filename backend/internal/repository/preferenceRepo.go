package repository

import "gorm.io/datatypes"

type Preferences struct{
	ID uint `gorm:"primaryKey"`
	ScheduledHunt datatypes.Date
	CreatedAt datatypes.Date
	UpdatedAt datatypes.Date
}