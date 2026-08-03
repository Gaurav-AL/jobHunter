package scheduler

import (
	"log"
	"time"

	"backend/internal/repository"
	"backend/internal/scraper"
	"backend/internal/services"

	"gorm.io/gorm"
)

// StartScheduler starts a simple loop that checks for scraping jobs
func StartScheduler(db *gorm.DB) {
	log.Println("Starting background scheduler...")
	
	// In a real app, use something like github.com/robfig/cron/v3
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			<-ticker.C
			runScrapingCycle(db)
		}
	}()
}

func runScrapingCycle(db *gorm.DB) {
	// 1. Get all allowed sources
	var sources []repository.AllowedSource
	if err := db.Find(&sources).Error; err != nil {
		log.Printf("Error fetching sources: %v", err)
		return
	}

	for _, source := range sources {
		// 2. Scrape Job
		job, err := scraper.ScrapeJob(source.URL)
		if err != nil {
			log.Printf("Failed to scrape %s: %v", source.URL, err)
			continue
		}
		job.SourceID = source.ID
		
		// Save job (ignore unique constraint errors for mockup)
		db.Create(job)

		// 3. Find interested users (for mockup, let's just get all users)
		var users []repository.User
		db.Find(&users)

		for _, user := range users {
			// Generate draft if resume is available
			// We skip actual file reading and use a dummy resume text if none is uploaded
			resumeText := "Software Developer with 3 years of Go and Vue experience."
			
			// Use LLM to generate Draft
			draftContent, err := services.GenerateDraft(resumeText, job.Description)
			if err != nil {
				log.Printf("LLM Generation failed for user %d: %v", user.ID, err)
				draftContent = "Error generating draft. Please review manually."
			}

			// Create Job Draft
			draft := repository.JobDraft{
				UserID:       user.ID,
				JobID:        job.ID,
				DraftContent: draftContent,
				Status:       "pending",
			}
			db.Create(&draft)

			// 4. Send Notification
			services.SendNotification(user.ID, user.Email, user.WhatsAppNumber, job.Title)
		}
	}
}
