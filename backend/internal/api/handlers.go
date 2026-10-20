package api

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"backend/internal/repository"
	"backend/internal/scheduler"
	"backend/internal/scraper"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const jobHunterProjectHighlight = "JobHunter — personal project: Go/Gin backend and Vue.js frontend with Docker Compose and MySQL; Firebase-authenticated user accounts; public Greenhouse, Lever, Ashby, Himalayas, and Jobicy job discovery; resume-based job matching and role-tailored resume/application drafts; scheduled email/SMS notifications. Review project details and metrics before using in an application."

func SetupRoutes(r *gin.Engine, db *gorm.DB) {
	api := r.Group("/api")
	api.Use(FirebaseAuth())

	// Users
	api.POST("/users/register", func(c *gin.Context) { RegisterUser(c, db) })
	api.POST("/users/resume", func(c *gin.Context) { UploadResume(c, db) })
	api.POST("/users/settings", func(c *gin.Context) { UpdateSettings(c, db) })
	api.POST("/users/phone/verify", func(c *gin.Context) { SendPhoneVerification(c, db) })
	api.POST("/users/phone/confirm", func(c *gin.Context) { ConfirmPhoneVerification(c, db) })
	api.GET("/users/profile", func(c *gin.Context) { GetProfile(c, db) })
	api.POST("/users/profile", func(c *gin.Context) { UpdateProfile(c, db) })
	api.GET("/dashboard", func(c *gin.Context) { GetDashboard(c, db) })
	api.GET("/integrations/health", func(c *gin.Context) { IntegrationHealth(c, db) })
	api.GET("/scheduler/status", func(c *gin.Context) { SchedulerStatus(c, db) })
	api.POST("/scheduler/run", func(c *gin.Context) { RunScheduler(c, db) })

	// Sources
	api.GET("/sources", func(c *gin.Context) { GetSources(c, db) })
	api.POST("/sources", func(c *gin.Context) { AddSource(c, db) })
	api.GET("/sources/discover", func(c *gin.Context) { DiscoverSources(c, db) })

	// Drafts
	api.GET("/drafts", func(c *gin.Context) { GetDrafts(c, db) })
	api.POST("/drafts/:id/status", func(c *gin.Context) { UpdateDraftStatus(c, db) })
	api.GET("/jobs/matches", func(c *gin.Context) { GetMatchedJobs(c, db) })
}

func resolveUserID(db *gorm.DB, userIDValue, firebaseUID string) (uint, error) {
	if userIDValue != "" {
		parsedID, err := strconv.ParseUint(userIDValue, 10, 64)
		if err != nil || parsedID == 0 {
			return 0, fmt.Errorf("invalid user_id")
		}
		return uint(parsedID), nil
	}

	if firebaseUID == "" {
		return 0, fmt.Errorf("firebase_uid is required")
	}

	var user repository.User
	if err := db.Where("firebase_uid = ?", firebaseUID).First(&user).Error; err != nil {
		return 0, fmt.Errorf("user not found for firebase_uid")
	}

	return user.ID, nil
}

func authenticatedUserID(c *gin.Context, db *gorm.DB) (uint, error) {
	uid := c.GetString("firebase_uid")
	if uid == "" {
		return 0, fmt.Errorf("authenticated Firebase user is required")
	}
	return resolveUserID(db, "", uid)
}

type RegisterRequest struct {
	Email       string `json:"email" binding:"required"`
	FirebaseUID string `json:"firebase_uid" binding:"required"`
}

func RegisterUser(c *gin.Context, db *gorm.DB) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.FirebaseUID != c.GetString("firebase_uid") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Firebase identity does not match the authenticated account"})
		return
	}
	user := repository.User{
		Email:       req.Email,
		FirebaseUID: c.GetString("firebase_uid"),
	}

	if err := db.Where("firebase_uid = ?", req.FirebaseUID).FirstOrCreate(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func UploadResume(c *gin.Context, db *gorm.DB) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<20)
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	file, err := c.FormFile("resume")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resume file is required"})
		return
	}

	allowedExts := map[string]bool{
		".pdf":  true,
		".doc":  true,
		".docx": true,
		".txt":  true,
		".md":   true,
		".png":  true,
		".jpg":  true,
		".jpeg": true,
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported resume file type"})
		return
	}

	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare upload directory"})
		return
	}

	filePath := fmt.Sprintf("%s/%d%s", uploadDir, userID, ext)

	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	resumeText, extractionWarning := extractResumeText(filePath, ext)
	profileSummary := ""
	var aiApplicantProfile services.ApplicantExtraction
	if resumeText != "" {
		if generatedProfile, err := services.ExtractApplicantProfile(resumeText); err == nil {
			aiApplicantProfile = generatedProfile
			profileSummary = generatedProfile.ProfileSummary
		} else {
			extractionWarning = strings.TrimSpace(extractionWarning + " AI applicant extraction unavailable: " + err.Error())
		}
	}

	updates := map[string]any{
		"resume_url":  filePath,
		"resume_text": resumeText,
	}
	// Structured extraction is sent to the user-configured AICC/Gemini provider.
	// Only blank fields are filled so reviewed profile values remain authoritative.
	if resumeText != "" {
		var existing repository.User
		if err := db.First(&existing, userID).Error; err == nil {
			applicantValues := applicantExtractionValues(aiApplicantProfile)
			for column, value := range extractApplicantDetailsLocal(resumeText) {
				if applicantValues[column] == "" {
					applicantValues[column] = value
				}
			}
			for column, value := range applicantValues {
				if strings.TrimSpace(value) != "" && applicantFieldEmpty(existing, column) {
					updates[column] = value
				}
			}
			if strings.TrimSpace(existing.ProfileSummary) != "" {
				delete(updates, "profile_summary")
			} else if aiApplicantProfile.ProfileSummary != "" {
				updates["profile_summary"] = aiApplicantProfile.ProfileSummary
			}
			if strings.TrimSpace(existing.TargetRoles) == "" && aiApplicantProfile.TargetRoles != "" {
				updates["target_roles"] = aiApplicantProfile.TargetRoles
			}
			if strings.TrimSpace(existing.Skills) == "" && aiApplicantProfile.Skills != "" {
				updates["skills"] = aiApplicantProfile.Skills
			}
			if strings.TrimSpace(existing.Location) == "" && aiApplicantProfile.Location != "" {
				updates["location"] = aiApplicantProfile.Location
			}
			if strings.TrimSpace(existing.ApplicantProjects) == "" {
				updates["applicant_projects"] = jobHunterProjectHighlight
			}
		}
	}

	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resume path"})
		return
	}

	var updatedUser repository.User
	_ = db.First(&updatedUser, userID).Error
	c.JSON(http.StatusOK, gin.H{
		"message":           "Resume uploaded successfully",
		"path":              filePath,
		"extracted_chars":   len(resumeText),
		"profile_summary":   profileSummary,
		"applicant_profile": profileResponse(updatedUser),
		"warning":           extractionWarning,
	})
}

type phoneVerificationRequest struct {
	Phone string `json:"phone" binding:"required"`
	Code  string `json:"code"`
}

func validE164(phone string) bool {
	return regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`).MatchString(strings.TrimSpace(phone))
}

func SendPhoneVerification(c *gin.Context, db *gorm.DB) {
	if _, err := authenticatedUserID(c, db); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var req phoneVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validE164(req.Phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enter a valid mobile number in international E.164 format, for example +14155552671"})
		return
	}
	if err := services.RequestPhoneVerification(strings.TrimSpace(req.Phone)); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Verification code requested. Check your SMS and enter the code here."})
}

func ConfirmPhoneVerification(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var req phoneVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validE164(req.Phone) || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid mobile number and verification code are required"})
		return
	}
	approved, err := services.CheckPhoneVerification(strings.TrimSpace(req.Phone), strings.TrimSpace(req.Code))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !approved {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "That verification code is incorrect or expired"})
		return
	}
	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(map[string]any{"whatsapp_number": strings.TrimSpace(req.Phone), "phone_verified": true}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save verified number"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Mobile number verified", "phone_verified": true})
}

type SettingsRequest struct {
	UserID        uint   `json:"user_id"`
	FirebaseUID   string `json:"firebase_uid"`
	CronFrequency string `json:"cron_frequency"`
	SMSOptIn      bool   `json:"sms_opt_in"`
}

func UpdateSettings(c *gin.Context, db *gorm.DB) {
	var req SettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user repository.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if req.CronFrequency != "hourly" && req.CronFrequency != "daily" && req.CronFrequency != "weekly" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cron_frequency must be hourly, daily, or weekly"})
		return
	}
	if req.SMSOptIn && (!user.PhoneVerified || user.WhatsAppNumber == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verify your mobile number before enabling SMS alerts"})
		return
	}
	if err := db.Model(&user).Updates(map[string]any{
		"cron_frequency": req.CronFrequency,
		"sms_opt_in":     req.SMSOptIn,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

func GetSources(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var sources []repository.AllowedSource
	if err := db.Where("is_global = ? OR user_id = ?", true, userID).Order("created_at DESC").Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sources"})
		return
	}
	c.JSON(http.StatusOK, sources)
}

func AddSource(c *gin.Context, db *gorm.DB) {
	var req struct {
		URL         string `json:"url"`
		URLUpper    string `json:"URL"`
		Name        string `json:"name"`
		NameUpper   string `json:"Name"`
		FirebaseUID string `json:"firebase_uid"`
		UserID      uint   `json:"user_id"`
		IsGlobal    bool   `json:"is_global"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sourceURL := strings.TrimSpace(req.URL)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(req.URLUpper)
	}
	if sourceURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	provider, boardToken, canonicalURL, parseErr := scraper.ParseBoardURL(sourceURL)
	if parseErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": parseErr.Error()})
		return
	}
	sourceURL = canonicalURL
	if req.IsGlobal {
		c.JSON(http.StatusForbidden, gin.H{"error": "Global source creation requires an administrator"})
		return
	}
	sourceName := strings.TrimSpace(req.Name)
	if sourceName == "" {
		sourceName = strings.TrimSpace(req.NameUpper)
	}
	if sourceName == "" {
		sourceName = sourceURL
	}

	ownerID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	userID := &ownerID

	source := repository.AllowedSource{
		URL:        sourceURL,
		Name:       sourceName,
		Provider:   provider,
		BoardToken: boardToken,
		IsGlobal:   false,
		UserID:     userID,
	}
	if err := db.Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create source. It may already exist."})
		return
	}
	c.JSON(http.StatusCreated, source)
}

type DraftView struct {
	ID             uint      `json:"ID"`
	JobID          uint      `json:"JobID"`
	DraftContent   string    `json:"DraftContent"`
	TailoredResume string    `json:"TailoredResume"`
	Status         string    `json:"Status"`
	JobTitle       string    `json:"JobTitle"`
	JobURL         string    `json:"JobURL"`
	ApplyURL       string    `json:"ApplyURL"`
	Location       string    `json:"Location"`
	Company        string    `json:"Company"`
	SourceName     string    `json:"SourceName"`
	JobActive      bool      `json:"JobActive"`
	LastSeenAt     time.Time `json:"LastSeenAt"`
}

type MatchedJobView struct {
	ID          uint       `json:"ID"`
	JobID       uint       `json:"JobID"`
	JobTitle    string     `json:"JobTitle"`
	JobURL      string     `json:"JobURL"`
	ApplyURL    string     `json:"ApplyURL"`
	Location    string     `json:"Location"`
	PublishedAt *time.Time `json:"PublishedAt"`
	LastSeenAt  time.Time  `json:"LastSeenAt"`
	JobActive   bool       `json:"JobActive"`
	Company     string     `json:"Company"`
	SourceName  string     `json:"SourceName"`
	SourceURL   string     `json:"SourceURL"`
	MatchScore  int        `json:"MatchScore"`
	MatchReason string     `json:"MatchReason"`
	Status      string     `json:"Status"`
}

func GetDrafts(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var drafts []repository.JobDraft
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&drafts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch drafts"})
		return
	}

	views := make([]DraftView, 0, len(drafts))
	for _, draft := range drafts {
		var job repository.Job
		var source repository.AllowedSource
		db.First(&job, draft.JobID)
		if job.SourceID != 0 {
			db.First(&source, job.SourceID)
		}
		company := job.Company
		if company == "" {
			company = source.Name
		}
		views = append(views, DraftView{
			ID:             draft.ID,
			JobID:          draft.JobID,
			DraftContent:   draft.DraftContent,
			TailoredResume: draft.TailoredResume,
			Status:         draft.Status,
			JobTitle:       job.Title,
			JobURL:         job.URL,
			ApplyURL:       job.ApplyURL,
			Location:       job.Location,
			Company:        company,
			SourceName:     source.Name,
			JobActive:      job.IsActive,
			LastSeenAt:     job.LastSeenAt,
		})
	}
	c.JSON(http.StatusOK, views)
}

func GetMatchedJobs(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var matches []repository.JobMatch
	if err := db.Where("user_id = ?", userID).Order("match_score DESC, created_at DESC").Find(&matches).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch matched jobs"})
		return
	}

	views := make([]MatchedJobView, 0, len(matches))
	for _, match := range matches {
		var job repository.Job
		var source repository.AllowedSource
		db.First(&job, match.JobID)
		if !job.IsActive {
			continue
		}
		if job.SourceID != 0 {
			db.First(&source, job.SourceID)
		}
		company := job.Company
		if company == "" {
			company = source.Name
		}
		views = append(views, MatchedJobView{
			ID:          match.ID,
			JobID:       match.JobID,
			JobTitle:    job.Title,
			JobURL:      job.URL,
			ApplyURL:    job.ApplyURL,
			Location:    job.Location,
			PublishedAt: job.PublishedAt,
			LastSeenAt:  job.LastSeenAt,
			JobActive:   job.IsActive,
			Company:     company,
			SourceName:  source.Name,
			SourceURL:   source.URL,
			MatchScore:  match.MatchScore,
			MatchReason: match.MatchReason,
			Status:      match.Status,
		})
	}
	c.JSON(http.StatusOK, views)
}

func UpdateDraftStatus(c *gin.Context, db *gorm.DB) {
	draftID := c.Param("id")
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != "applied" && req.Status != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be applied or rejected"})
		return
	}
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	if req.Status == "applied" {
		var draft repository.JobDraft
		if err := db.Where("id = ? AND user_id = ?", draftID, userID).First(&draft).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
			return
		}
		var job repository.Job
		if err := db.First(&job, draft.JobID).Error; err != nil || !job.IsActive || job.ApplyURL == "" {
			c.JSON(http.StatusConflict, gin.H{"error": "This listing is no longer confirmed active or has no application URL"})
			return
		}
	}
	result := db.Model(&repository.JobDraft{}).Where("id = ? AND user_id = ?", draftID, userID).Update("status", req.Status)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

func IntegrationHealth(c *gin.Context, db *gorm.DB) {
	sqlDB, err := db.DB()
	dbStatus := "ok"
	if err != nil {
		dbStatus = "unavailable"
	} else if err := sqlDB.Ping(); err != nil {
		dbStatus = "unreachable"
	}

	c.JSON(http.StatusOK, gin.H{
		"database": gin.H{
			"status": dbStatus,
		},
		"llm": gin.H{
			"provider": "aicc",
			"api_key":  os.Getenv("AICC_KEY") != "",
		},
	})
}

func GetProfile(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user repository.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, profileResponse(user))
}

func UpdateProfile(c *gin.Context, db *gorm.DB) {
	var req struct {
		UserID              uint   `json:"user_id"`
		FirebaseUID         string `json:"firebase_uid"`
		ProfileSummary      string `json:"profile_summary"`
		TargetRoles         string `json:"target_roles"`
		Skills              string `json:"skills"`
		Location            string `json:"location"`
		CronFrequency       string `json:"cron_frequency"`
		ApplicantFirstName  string `json:"applicant_first_name"`
		ApplicantLastName   string `json:"applicant_last_name"`
		ApplicantPhone      string `json:"applicant_phone"`
		ApplicantAddress    string `json:"applicant_address"`
		ApplicantCity       string `json:"applicant_city"`
		ApplicantRegion     string `json:"applicant_region"`
		ApplicantPostalCode string `json:"applicant_postal_code"`
		ApplicantCountry    string `json:"applicant_country"`
		ApplicantLinkedIn   string `json:"applicant_linkedin"`
		ApplicantWebsite    string `json:"applicant_website"`
		ApplicantGitHub     string `json:"applicant_github"`
		ApplicantProjects   string `json:"applicant_projects"`
		WorkAuthorization   string `json:"work_authorization"`
		VisaSponsorship     string `json:"visa_sponsorship"`
		NoticePeriod        string `json:"notice_period"`
		SalaryExpectation   string `json:"salary_expectation"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]any{
		"profile_summary":       req.ProfileSummary,
		"target_roles":          req.TargetRoles,
		"skills":                req.Skills,
		"location":              req.Location,
		"applicant_first_name":  req.ApplicantFirstName,
		"applicant_last_name":   req.ApplicantLastName,
		"applicant_phone":       req.ApplicantPhone,
		"applicant_address":     req.ApplicantAddress,
		"applicant_city":        req.ApplicantCity,
		"applicant_region":      req.ApplicantRegion,
		"applicant_postal_code": req.ApplicantPostalCode,
		"applicant_country":     req.ApplicantCountry,
		"applicant_linkedin":    req.ApplicantLinkedIn,
		"applicant_website":     req.ApplicantWebsite,
		"applicant_github":      req.ApplicantGitHub,
		"applicant_projects":    req.ApplicantProjects,
		"work_authorization":    req.WorkAuthorization,
		"visa_sponsorship":      req.VisaSponsorship,
		"notice_period":         req.NoticePeriod,
		"salary_expectation":    req.SalaryExpectation,
	}
	if req.CronFrequency != "" {
		updates["cron_frequency"] = req.CronFrequency
	}
	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}
	var user repository.User
	db.First(&user, userID)
	c.JSON(http.StatusOK, profileResponse(user))
}

func GetDashboard(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user repository.User
	db.First(&user, userID)
	if strings.TrimSpace(user.ApplicantProjects) == "" {
		user.ApplicantProjects = jobHunterProjectHighlight
		db.Model(&repository.User{}).Where("id = ?", userID).Update("applicant_projects", user.ApplicantProjects)
	}
	var sources []repository.AllowedSource
	db.Where("is_global = ? OR user_id = ?", true, userID).Order("created_at DESC").Find(&sources)
	var drafts []repository.JobDraft
	db.Where("user_id = ?", userID).Order("created_at DESC").Find(&drafts)
	var jobsCount int64
	db.Model(&repository.Job{}).Count(&jobsCount)
	var matchedJobs []MatchedJobView
	var matches []repository.JobMatch
	db.Where("user_id = ?", userID).Order("match_score DESC, created_at DESC").Limit(20).Find(&matches)
	for _, match := range matches {
		var job repository.Job
		var source repository.AllowedSource
		db.First(&job, match.JobID)
		if !job.IsActive {
			continue
		}
		if job.SourceID != 0 {
			db.First(&source, job.SourceID)
		}
		company := job.Company
		if company == "" {
			company = source.Name
		}
		matchedJobs = append(matchedJobs, MatchedJobView{
			ID:          match.ID,
			JobID:       match.JobID,
			JobTitle:    job.Title,
			JobURL:      job.URL,
			ApplyURL:    job.ApplyURL,
			Location:    job.Location,
			PublishedAt: job.PublishedAt,
			LastSeenAt:  job.LastSeenAt,
			JobActive:   job.IsActive,
			Company:     company,
			SourceName:  source.Name,
			SourceURL:   source.URL,
			MatchScore:  match.MatchScore,
			MatchReason: match.MatchReason,
			Status:      match.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"user":             profileResponse(user),
		"sources":          sources,
		"drafts_count":     len(drafts),
		"jobs_count":       jobsCount,
		"matched_jobs":     matchedJobs,
		"scheduler_status": scheduler.GetStatus(),
		"integrations": gin.H{
			"aicc_key": os.Getenv("AICC_KEY") != "",
		},
	})
}

func SchedulerStatus(c *gin.Context, db *gorm.DB) {
	c.JSON(http.StatusOK, scheduler.GetStatus())
}

func RunScheduler(c *gin.Context, db *gorm.DB) {
	go scheduler.RunScrapingCycle(db)
	c.JSON(http.StatusAccepted, gin.H{"message": "Job hunt started"})
}

func DiscoverSources(c *gin.Context, db *gorm.DB) {
	c.JSON(http.StatusOK, scraper.DiscoverSources(c.Query("query")))
}

func profileResponse(user repository.User) gin.H {
	return gin.H{
		"id":                    user.ID,
		"email":                 user.Email,
		"resume_url":            user.ResumeURL,
		"resume_text":           user.ResumeText,
		"profile_summary":       user.ProfileSummary,
		"target_roles":          user.TargetRoles,
		"skills":                user.Skills,
		"location":              user.Location,
		"applicant_first_name":  user.ApplicantFirstName,
		"applicant_last_name":   user.ApplicantLastName,
		"applicant_phone":       user.ApplicantPhone,
		"applicant_address":     user.ApplicantAddress,
		"applicant_city":        user.ApplicantCity,
		"applicant_region":      user.ApplicantRegion,
		"applicant_postal_code": user.ApplicantPostalCode,
		"applicant_country":     user.ApplicantCountry,
		"applicant_linkedin":    user.ApplicantLinkedIn,
		"applicant_website":     user.ApplicantWebsite,
		"applicant_github":      user.ApplicantGitHub,
		"applicant_projects":    user.ApplicantProjects,
		"work_authorization":    user.WorkAuthorization,
		"visa_sponsorship":      user.VisaSponsorship,
		"notice_period":         user.NoticePeriod,
		"salary_expectation":    user.SalaryExpectation,
		"cron_frequency":        user.CronFrequency,
		"whatsapp_number":       user.WhatsAppNumber,
		"phone_verified":        user.PhoneVerified,
		"sms_opt_in":            user.SMSOptIn,
	}
}

func extractApplicantDetailsLocal(text string) map[string]string {
	result := map[string]string{}
	patterns := map[string]string{
		"applicant_phone":    `(?i)(?:\+91[\s().-]*)?[6-9][0-9](?:[\s().-]*[0-9]){8}`,
		"applicant_linkedin": `(?i)https?://(?:www\.)?linkedin\.com/in/[a-z0-9_%-]+/?`,
		"applicant_github":   `(?i)https?://(?:www\.)?github\.com/[a-z0-9-]+/?`,
	}
	for field, pattern := range patterns {
		if match := regexp.MustCompile(pattern).FindString(text); match != "" {
			result[field] = strings.TrimSpace(match)
		}
	}
	return result
}

func applicantExtractionValues(extracted services.ApplicantExtraction) map[string]string {
	values := map[string]string{
		"applicant_first_name":  extracted.ApplicantFirstName,
		"applicant_last_name":   extracted.ApplicantLastName,
		"applicant_phone":       extracted.ApplicantPhone,
		"applicant_address":     extracted.ApplicantAddress,
		"applicant_city":        extracted.ApplicantCity,
		"applicant_region":      extracted.ApplicantRegion,
		"applicant_postal_code": extracted.ApplicantPostalCode,
		"applicant_country":     extracted.ApplicantCountry,
		"applicant_linkedin":    extracted.ApplicantLinkedIn,
		"applicant_website":     extracted.ApplicantWebsite,
		"applicant_github":      extracted.ApplicantGitHub,
	}
	return values
}

func applicantFieldEmpty(user repository.User, column string) bool {
	values := map[string]string{
		"applicant_first_name":  user.ApplicantFirstName,
		"applicant_last_name":   user.ApplicantLastName,
		"applicant_phone":       user.ApplicantPhone,
		"applicant_address":     user.ApplicantAddress,
		"applicant_city":        user.ApplicantCity,
		"applicant_region":      user.ApplicantRegion,
		"applicant_postal_code": user.ApplicantPostalCode,
		"applicant_country":     user.ApplicantCountry,
		"applicant_linkedin":    user.ApplicantLinkedIn,
		"applicant_website":     user.ApplicantWebsite,
		"applicant_github":      user.ApplicantGitHub,
	}
	return strings.TrimSpace(values[column]) == ""
}

func extractResumeText(filePath, ext string) (string, string) {
	switch ext {
	case ".txt", ".md", ".csv":
		bytes, err := os.ReadFile(filePath)
		if err != nil {
			return "", "Could not read uploaded resume text."
		}
		return strings.TrimSpace(string(bytes)), ""
	case ".docx":
		text, err := extractDocxText(filePath)
		if err != nil {
			return "", "Could not extract DOCX text: " + err.Error()
		}
		return text, ""
	case ".pdf":
		text, err := extractPDFText(filePath)
		if err != nil {
			return "", "PDF upload is saved, but text extraction failed: " + err.Error()
		}
		return text, ""
	case ".png", ".jpg", ".jpeg":
		return "", "Image resume upload is saved, but OCR is not installed in this container yet. Upload .txt or .docx for profile extraction."
	default:
		return "", "Unsupported extraction type."
	}
}

func extractPDFText(filePath string) (string, error) {
	output, err := exec.Command("pdftotext", "-layout", filePath, "-").Output()
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(string(output), " "))
	if text == "" {
		return "", fmt.Errorf("pdftotext returned no text; this PDF may be scanned or image-only")
	}
	return text, nil
}

func extractDocxText(filePath string) (string, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		bytes, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		decoder := xml.NewDecoder(strings.NewReader(string(bytes)))
		var parts []string
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if charData, ok := token.(xml.CharData); ok {
				text := strings.TrimSpace(string(charData))
				if text != "" {
					parts = append(parts, text)
				}
			}
		}
		collapsed := regexp.MustCompile(`\s+`).ReplaceAllString(strings.Join(parts, " "), " ")
		return strings.TrimSpace(collapsed), nil
	}
	return "", fmt.Errorf("word/document.xml not found")
}
