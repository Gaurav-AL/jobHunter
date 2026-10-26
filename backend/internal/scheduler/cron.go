package scheduler

import (
	"log"
	"os"
	"strconv"
	"strings"
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
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DISABLE_SCHEDULER")), "true") {
		statusMu.Lock()
		status.Running = false
		status.LastRunMessage = "Background scheduler is disabled for this deployment."
		status.NextRunHint = "Use Run now to refresh supported job boards."
		statusMu.Unlock()
		log.Println("Background job discovery scheduler disabled by configuration")
		return
	}
	statusMu.Lock()
	status.Running = true
	statusMu.Unlock()
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
	return status
}

func RunScrapingCycle(db *gorm.DB) {
	statusMu.Lock()
	status.LastRunAt = time.Now()
	status.LastRunMessage = "Checking published job boards..."
	statusMu.Unlock()
	discoveredJobs, matchedJobs := 0, 0
	var sources []repository.AllowedSource
	if err := db.Order("CASE WHEN provider = 'greenhouse' THEN 0 WHEN provider IN ('lever', 'ashby') THEN 1 WHEN provider IN ('himalayas', 'jobicy') THEN 2 WHEN provider IN ('remoteok', 'arbeitnow') THEN 3 ELSE 4 END").Find(&sources).Error; err != nil {
		log.Printf("Error fetching sources: %v", err)
		updateStatus("Failed to fetch sources.")
		return
	}
	// Track which matches were created/updated this cycle so we can auto-draft them.
	type matchKey struct{ userID, jobID uint }
	freshMatches := map[matchKey]bool{}

	for _, source := range sources {
		jobs, err := scraper.ScrapeJobs(source.URL)
		if err != nil {
			log.Printf("Failed to fetch source %s: %v", source.URL, err)
			continue
		}
		seenURLs := make([]string, 0, len(jobs))
		for _, job := range jobs {
			job.SourceID = source.ID
			job.Provider = source.Provider
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
				if err = db.Model(&existing).Updates(map[string]any{"source_id": source.ID, "provider": job.Provider, "external_id": job.ExternalID, "company": job.Company, "title": job.Title, "description": job.Description, "apply_url": job.ApplyURL, "location": job.Location, "published_at": job.PublishedAt, "last_seen_at": job.LastSeenAt, "scraped_at": job.ScrapedAt, "is_active": true}).Error; err != nil {
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
				skillNames := strings.Join(match.MatchedSkills, ", ")
				requiredSkills := strings.Join(services.JobSkillsAsked(existing), ", ")
				result := db.Where("user_id = ? AND job_id = ?", user.ID, existing.ID).FirstOrCreate(&matchRecord, repository.JobMatch{UserID: user.ID, JobID: existing.ID, MatchScore: match.Score, MatchReason: match.Reason, MatchedSkills: skillNames, RequiredSkills: requiredSkills, Status: "not_applied"})
				if result.Error != nil {
					log.Printf("Failed to save job match: %v", result.Error)
					continue
				}
				isNewMatch := result.RowsAffected > 0
				if !isNewMatch {
					db.Model(&matchRecord).Updates(map[string]any{"match_score": match.Score, "match_reason": match.Reason, "matched_skills": skillNames, "required_skills": requiredSkills})
				} else {
					matchedJobs++
					// Mark this match for auto-draft generation only if it is a brand new match
					freshMatches[matchKey{user.ID, existing.ID}] = true
					sendJobNotification(db, user, source, existing)
				}
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

	// Auto-generate application packs for fresh top matches so the dashboard
	// is ready for review the moment the user opens it via the email link.
	autoDraftCount := 0
	for key := range freshMatches {
		var user repository.User
		if err := db.First(&user, key.userID).Error; err != nil || strings.TrimSpace(user.ResumeText) == "" {
			continue // Skip if no resume — draft would be a fallback only
		}
		var job repository.Job
		if err := db.First(&job, key.jobID).Error; err != nil || !job.IsActive {
			continue
		}
		// Only draft if an apply link is available.
		if job.ApplyURL == "" && job.URL == "" {
			continue
		}
		// Skip if a draft already exists (don't overwrite user edits).
		var existingDraft repository.JobDraft
		if db.Where("user_id = ? AND job_id = ?", key.userID, key.jobID).First(&existingDraft).Error == nil {
			continue
		}
		profileFacts := "Notice period: " + user.NoticePeriod + "\nExperience: " + user.ApplicantYearsExperience + "\nExpected compensation: " + user.SalaryExpectation + "\nApplicant projects: " + user.ApplicantProjects
		resumeForDraft := user.ResumeText + "\n\n" + profileFacts
		jobContext := "Title: " + job.Title + "\nCompany: " + job.Company + "\nLocation: " + job.Location + "\n\nDescription:\n" + job.Description

		cover, coverErr := services.GenerateDraft(resumeForDraft, jobContext)
		if coverErr != nil {
			cover = buildFallbackMessage(user, job)
		}
		tailored, resumeErr := services.GenerateTailoredResume(user.ResumeText, job.Title, job.Description)
		if resumeErr != nil {
			tailored = services.BuildRoleFocusedResume(user, job.Title, job.Description)
		}

		draft := repository.JobDraft{
			UserID:         key.userID,
			JobID:          key.jobID,
			DraftContent:   cover,
			TailoredResume: tailored,
			Status:         "pending",
		}
		if err := db.Create(&draft).Error; err != nil {
			log.Printf("Auto-draft creation failed for user %d job %d: %v", key.userID, key.jobID, err)
			continue
		}
		autoDraftCount++
		log.Printf("Auto-drafted application pack for user %d job %d (%s)", key.userID, key.jobID, job.Title)
	}

	msg := logMessage(discoveredJobs, matchedJobs)
	if autoDraftCount > 0 {
		msg += " Auto-prepared " + strconv.Itoa(autoDraftCount) + " application pack(s) ready for your review."
	}
	updateStatus(msg)
}

func buildFallbackMessage(user repository.User, job repository.Job) string {
	var sb strings.Builder
	sb.WriteString("Application note — " + job.Title + " at " + job.Company + "\n\n")
	sb.WriteString("I am interested in this role and would welcome the opportunity to discuss how my background aligns with the position. The attached resume contains my verified experience and skills.\n\n")
	if user.ApplicantFirstName != "" {
		sb.WriteString("Regards,\n" + user.ApplicantFirstName + " " + user.ApplicantLastName)
	}
	return sb.String()
}

func logMessage(discoveredJobs, matchedJobs int) string {
	return "Listings currently published: " + strconv.Itoa(discoveredJobs) + ", profile matches: " + strconv.Itoa(matchedJobs) + ". No applications were submitted because employer-authorized ATS access is not configured."
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
