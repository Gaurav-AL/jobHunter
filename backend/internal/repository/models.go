package repository

import (
	"time"

	"gorm.io/gorm"
)

// User represents the system user
type User struct {
	ID                       uint   `gorm:"primaryKey"`
	Email                    string `gorm:"size:255;uniqueIndex;not null"`
	FirebaseUID              string `gorm:"size:255;uniqueIndex;not null"`
	ResumeURL                string `gorm:"size:512"`      // URL to stored resume (e.g., local path or cloud storage)
	ResumeText               string `gorm:"type:longtext"` // Extracted resume text used for matching and drafting
	ProfileSummary           string `gorm:"type:text"`     // AI/user edited professional summary
	TargetRoles              string `gorm:"type:text"`     // Comma/newline separated target roles
	Skills                   string `gorm:"type:text"`     // Comma/newline separated skills
	Location                 string `gorm:"size:255"`      // Preferred location or remote preference
	ApplicantFirstName       string `gorm:"size:120"`
	ApplicantLastName        string `gorm:"size:120"`
	ApplicantPhone           string `gorm:"size:32"`
	ApplicantAddress         string `gorm:"size:512"`
	ApplicantCity            string `gorm:"size:120"`
	ApplicantRegion          string `gorm:"size:120"`
	ApplicantPostalCode      string `gorm:"size:32"`
	ApplicantCountry         string `gorm:"size:120"`
	ApplicantLinkedIn        string `gorm:"column:applicant_linkedin;size:512"`
	ApplicantWebsite         string `gorm:"size:512"`
	ApplicantGitHub          string `gorm:"column:applicant_github;size:512"`
	ApplicantProjects        string `gorm:"type:longtext"`
	WorkAuthorization        string `gorm:"size:255"`
	VisaSponsorship          string `gorm:"size:255"`
	NoticePeriod             string `gorm:"size:120"`
	ApplicantYearsExperience string `gorm:"size:32"`
	SalaryExpectation        string `gorm:"size:120"`
	CronFrequency            string `gorm:"size:32;default:'daily'"` // e.g., 'hourly', 'daily', 'weekly'
	WhatsAppNumber           string `gorm:"size:32"`                 // Reused field for verified SMS recipient
	PhoneVerified            bool   `gorm:"not null;default:false"`
	SMSOptIn                 bool   `gorm:"not null;default:false"`
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}

// AllowedSource represents a career page to scrape
type AllowedSource struct {
	ID         uint   `gorm:"primaryKey"`
	URL        string `gorm:"size:512;uniqueIndex;not null"`
	Name       string `gorm:"size:255"`
	Provider   string `gorm:"size:32;not null;default:'greenhouse'"`
	BoardToken string `gorm:"size:128;not null"`
	IsGlobal   bool   `gorm:"default:false"` // If true, applies to all users.
	UserID     *uint  // If not global, belongs to a specific user
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Job represents a job found by scraping
type NotificationDelivery struct {
	ID                uint   `gorm:"primaryKey"`
	UserID            uint   `gorm:"not null;index"`
	JobID             uint   `gorm:"not null;index"`
	Channel           string `gorm:"size:16;not null"`
	Status            string `gorm:"size:32;not null"`
	ProviderMessageID string `gorm:"size:64"`
	Error             string `gorm:"size:512"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Job struct {
	ID          uint   `gorm:"primaryKey"`
	SourceID    uint   `gorm:"not null"`
	Company     string `gorm:"size:255"`
	Title       string `gorm:"size:255;not null"`
	Description string `gorm:"type:text"`
	URL         string `gorm:"size:512;uniqueIndex;not null"`
	ApplyURL    string `gorm:"size:1024"`
	ExternalID  string `gorm:"size:255;index"`
	Location    string `gorm:"size:255"`
	PublishedAt *time.Time
	LastSeenAt  time.Time `gorm:"index"`
	IsActive    bool      `gorm:"default:true;index"`
	ScrapedAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// JobMatch records a job that passed the resume/profile relevance filter.
type JobMatch struct {
	ID          uint   `gorm:"primaryKey"`
	UserID      uint   `gorm:"not null;index:idx_user_job_match,unique"`
	JobID       uint   `gorm:"not null;index:idx_user_job_match,unique"`
	MatchScore  int    `gorm:"not null"`
	MatchReason string `gorm:"type:text"`
	Status      string `gorm:"size:32;default:'matched'"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// JobDraft represents a personalized draft for a user based on a job
type JobDraft struct {
	ID             uint   `gorm:"primaryKey"`
	UserID         uint   `gorm:"not null"`
	JobID          uint   `gorm:"not null"`
	DraftContent   string `gorm:"type:longtext"` // Tailored cover letter/application message
	TailoredResume string `gorm:"type:longtext"`
	Status         string `gorm:"size:32;default:'pending';index"` // pending, applied, rejected
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
