package repository

import "time"


type Jobs struct{
	ID uint `gorm:"primaryKey"`
	Salary string
	Title string
	Eligibility string
	Responsibility string
	ExperienceRequired string
	Country string
	State string
	City string
	CreatedAt time.Time
	UpdatedAt time.Time
}