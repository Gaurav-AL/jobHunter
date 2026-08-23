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
	status   = Status{
		Running:        false,
		LastRunMessage: "Scheduler has not run yet.",
		NextRunHint:    "Runs every 1 minute in this prototype.",
	}
)

// StartScheduler starts a simple loop that checks for scraping jobs
func StartScheduler(db *gorm.DB) {
	log.Println("Starting background scheduler...")

	// In a real app, use something like github.com/robfig/cron/v3
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			<-ticker.C
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
	status.LastRunMessage = "Running job hunt..."
	statusMu.Unlock()

	discoveredJobs := 0
	matchedJobs := 0
	createdDrafts := 0

	// 1. Get all allowed sources
	var sources []repository.AllowedSource
	if err := db.Find(&sources).Error; err != nil {
		log.Printf("Error fetching sources: %v", err)
		updateStatus("Failed to fetch sources: " + err.Error())
		return
	}

	for _, source := range sources {
		// 2. Scrape Job
		jobs, err := scraper.ScrapeJobs(source.URL)
		if err != nil {
			log.Printf("Failed to scrape %s: %v", source.URL, err)
			continue
		}

		// 3. Find interested users. Global sources apply to all users; personal sources apply to the owner.
		var users []repository.User
		if source.IsGlobal || source.UserID == nil {
			db.Find(&users)
		} else {
			db.Where("id = ?", *source.UserID).Find(&users)
		}

		for _, job := range jobs {
			job.SourceID = source.ID
			var existing repository.Job
			if err := db.Where("url = ?", job.URL).FirstOrCreate(&existing, job).Error; err != nil {
				log.Printf("Failed to save job %s: %v", job.URL, err)
				continue
			}
			discoveredJobs++

			for _, user := range users {
				match := services.MatchJobToProfile(user, existing)
				if match.Score < 25 {
					log.Printf("Skipping low-match job %d for user %d: score=%d", existing.ID, user.ID, match.Score)
					continue
				}
				var existingMatch repository.JobMatch
				db.Where("user_id = ? AND job_id = ?", user.ID, existing.ID).FirstOrCreate(&existingMatch, repository.JobMatch{
					UserID:      user.ID,
					JobID:       existing.ID,
					MatchScore:  match.Score,
					MatchReason: match.Reason,
					Status:      "matched",
				})
				if existingMatch.ID != 0 {
					db.Model(&existingMatch).Updates(repository.JobMatch{
						MatchScore:  match.Score,
						MatchReason: match.Reason,
					})
				}
				matchedJobs++

				var existingDraft repository.JobDraft
				if err := db.Where("user_id = ? AND job_id = ?", user.ID, existing.ID).First(&existingDraft).Error; err == nil {
					continue
				}

				resumeText := buildProfileText(user)

				draftContent, err := services.GenerateDraft(resumeText, existing.Description)
				if err != nil {
					log.Printf("AICC generation failed for user %d: %v", user.ID, err)
					draftContent = "A matching job was discovered. AICC draft generation is unavailable right now, so please review the job manually."
				}

				draft := repository.JobDraft{
					UserID:       user.ID,
					JobID:        existing.ID,
					DraftContent: draftContent,
					Status:       "pending",
				}
				if err := db.Create(&draft).Error; err == nil {
					createdDrafts++
				}

				// 4. Send Notification
				services.SendNotification(user.ID, user.Email, user.WhatsAppNumber, existing.Title)
			}
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
	if text == "" {
		text = "Candidate profile is incomplete. Generate a concise review note and ask the user to complete profile personalization."
	}
	return text
}

func logMessage(discoveredJobs, matchedJobs, createdDrafts int) string {
	return "Last run completed. Jobs discovered: " + strconv.Itoa(discoveredJobs) + ", matches kept: " + strconv.Itoa(matchedJobs) + ", drafts created: " + strconv.Itoa(createdDrafts) + "."
}

func updateStatus(message string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	status.LastRunAt = time.Now()
	status.LastRunMessage = message
}
