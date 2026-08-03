package repository

import (
    "gorm.io/datatypes"
)

type WorkExperience struct{
	ID uint `gorm:"primaryKey"`
	Role string
	StartDate datatypes.Date
	EndDate datatypes.Date
	CompanyName string
	Description string
}

type Resume struct{
	ID uint `gorm:"primaryKey"`
	FullName string
	Role string
	PhoneNumber string
	Address string
	SocialLinks []string
	WorkExperiences []WorkExperience
	Skills datatypes.JSON
	Projects datatypes.JSON
	Certifications datatypes.JSON
	Achievements datatypes.JSON
	CreatedAt datatypes.Date
	UpdatedAt datatypes.Date
}