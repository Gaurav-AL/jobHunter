package scheduler

import (
	"log"
	"strconv"
	"sync"
	"time"

	"backend/internal/repository"
	"backend/internal/scraper"
	"backend/internal/services"

	"gorm.io/gorm"
)

type Status struct {
	Running        bool      `json:"running"`
	LastRunAt      time.Time `json:"last_run_at"`
	LastRunMessage string    `json:"last_run_message"`
	NextRunHint    string    `json:"next_run_hint"`
}

var (
	statusMu sync.RWMutex
	status   = Status{Running: false, LastRunMessage: "Scheduler has not run yet.", NextRunHint: "Checks supported ATS job boards every 6 hours."}
)

func StartScheduler(db *gorm.DB) {
	log.Println("Starting job discovery scheduler")
	ticker := time.NewTicker(6 * time.Hour)
	go func() {
		for range ticker.C {
			RunScrapingCycle(db)
		}
	}()
}

func GetStatus() Status {
	statusMu.RLock()
	defer statusMu.RUnlock()
	current := status
	current.Running = true
	return current
}

func RunScrapingCycle(db *gorm.DB) {
	statusMu.Lock()
	status.LastRunAt = time.Now()
	status.LastRunMessage = "Checking published job boards..."
	statusMu.Unlock()
	discoveredJobs, matchedJobs, createdDrafts := 0, 0, 0
	var sources []repository.AllowedSource
	if err := db.Find(&sources).Error; err != nil {
		log.Printf("Error fetching sources: %v", err)
		updateStatus("Failed to fetch sources.")
		return
	}
	for _, source := range sources {
		jobs, err := scraper.ScrapeJobs(source.URL)
		if err != nil {
			log.Printf("Failed to fetch source %s: %v", source.URL, err)
			continue
		}
		seenURLs := make([]string, 0, len(jobs))
		for _, job := range jobs {
			job.SourceID = source.ID
			seenURLs = append(seenURLs, job.URL)
			var existing repository.Job
			err := db.Where("url = ?", job.URL).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err = db.Create(job).Error; err != nil {
					log.Printf("Failed to insert job %s: %v", job.URL, err)
					continue
				}
				existing = *job
			} else if err != nil {
				log.Printf("Failed to look up job %s: %v", job.URL, err)
				continue
			} else {
				jobID := existing.ID
				if err = db.Model(&existing).Updates(map[string]any{"source_id": source.ID, "external_id": job.ExternalID, "title": job.Title, "description": job.Description, "apply_url": job.ApplyURL, "location": job.Location, "published_at": job.PublishedAt, "last_seen_at": job.LastSeenAt, "scraped_at": job.ScrapedAt, "is_active": true}).Error; err != nil {
					log.Printf("Failed to refresh job %s: %v", job.URL, err)
					continue
				}
				existing = *job
				existing.ID = jobID
			}
			discoveredJobs++
			var users []repository.User
			if source.IsGlobal || source.UserID == nil {
				if err := db.Find(&users).Error; err != nil {
					log.Printf("Failed to read user profiles: %v", err)
					continue
				}
			} else if err := db.Where("id = ?", *source.UserID).Find(&users).Error; err != nil {
				log.Printf("Failed to read source owner profile: %v", err)
				continue
			}
			for _, user := range users {
				match := services.MatchJobToProfile(user, existing)
				if match.Score < 25 {
					continue
				}
				var matchRecord repository.JobMatch
				result := db.Where("user_id = ? AND job_id = ?", user.ID, existing.ID).FirstOrCreate(&matchRecord, repository.JobMatch{UserID: user.ID, JobID: existing.ID, MatchScore: match.Score, MatchReason: match.Reason, Status: "matched"})
				if result.Error != nil {
					log.Printf("Failed to save job match: %v", result.Error)
					continue
				}
				if result.RowsAffected == 0 {
					db.Model(&matchRecord).Updates(map[string]any{"match_score": match.Score, "match_reason": match.Reason})
				}
				matchedJobs++
				var draft repository.JobDraft
				draftErr := db.Where("user_id = ? AND job_id = ?", user.ID, existing.ID).First(&draft).Error
				if draftErr == gorm.ErrRecordNotFound {
					profileText := buildProfileText(user)
					coverLetter, coverErr := services.GenerateDraft(profileText, existing.Description)
					if coverErr != nil {
						log.Printf("Cover letter generation unavailable for user %d job %d: %v", user.ID, existing.ID, coverErr)
						coverLetter = "Review this official job posting and write a concise application note using only verified details from your resume."
					}
					roleResume, resumeErr := services.GenerateTailoredResume(profileText, existing.Title, existing.Description)
					if resumeErr != nil {
						log.Printf("Resume tailoring unavailable for user %d job %d: %v", user.ID, existing.ID, resumeErr)
						roleResume = services.BuildRoleFocusedResume(user, existing.Title, existing.Description)
					}
					draft = repository.JobDraft{UserID: user.ID, JobID: existing.ID, DraftContent: coverLetter, TailoredResume: roleResume, Status: "pending"}
					if err := db.Create(&draft).Error; err != nil {
						log.Printf("Failed to create draft: %v", err)
						continue
					}
					createdDrafts++
				} else if draftErr != nil {
					log.Printf("Failed to load draft for user %d job %d: %v", user.ID, existing.ID, draftErr)
					continue
				}
				sendJobNotification(db, user, source, existing)
			}
		}
		// Only a successful ATS response can close jobs that disappeared from the public board.
		query := db.Model(&repository.Job{}).Where("source_id = ?", source.ID)
		if len(seenURLs) > 0 {
			query = query.Where("url NOT IN ?", seenURLs)
		}
		if err := query.Update("is_active", false).Error; err != nil {
			log.Printf("Failed to close stale jobs for source %d: %v", source.ID, err)
		}
	}
	updateStatus(logMessage(discoveredJobs, matchedJobs, createdDrafts))
}

func buildProfileText(user repository.User) string {
	text := user.ResumeText
	if user.ProfileSummary != "" {
		text += "\nProfile Summary: " + user.ProfileSummary
	}
	if user.TargetRoles != "" {
		text += "\nTarget Roles: " + user.TargetRoles
	}
	if user.Skills != "" {
		text += "\nSkills: " + user.Skills
	}
	if user.Location != "" {
		text += "\nLocation Preference: " + user.Location
	}
	return text
}

func logMessage(discoveredJobs, matchedJobs, createdDrafts int) string {
	return "ATS listings currently published: " + strconv.Itoa(discoveredJobs) + ", profile matches: " + strconv.Itoa(matchedJobs) + ", new resume/application drafts: " + strconv.Itoa(createdDrafts) + "."
}

func updateStatus(message string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	status.LastRunAt = time.Now()
	status.LastRunMessage = message
}

func sendJobNotification(db *gorm.DB, user repository.User, source repository.AllowedSource, job repository.Job) {
	if !user.SMSOptIn || !user.PhoneVerified || user.WhatsAppNumber == "" {
		return
	}
	var previous repository.NotificationDelivery
	err := db.Where("user_id = ? AND job_id = ? AND channel = ?", user.ID, job.ID, "sms").Order("created_at DESC").First(&previous).Error
	if err == nil && previous.Status != "failed" && previous.Status != "undelivered" && previous.Status != "not_configured" {
		return
	}
	status, providerID, sendErr := services.SendJobSMS(user.WhatsAppNumber, source.Name, job.Title, job.ApplyURL)
	delivery := repository.NotificationDelivery{UserID: user.ID, JobID: job.ID, Channel: "sms", Status: status, ProviderMessageID: providerID}
	if sendErr != nil {
		delivery.Error = sendErr.Error()
		log.Printf("SMS notification failed for user %d: %v", user.ID, sendErr)
	}
	if err := db.Create(&delivery).Error; err != nil {
		log.Printf("Failed to record SMS delivery status: %v", err)
	}
}
