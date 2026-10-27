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

const jobHunterProjectHighlight = "JobHunter — personal project: Go/Gin backend and Vue.js frontend with Docker Compose and MySQL; Firebase-authenticated user accounts; public Greenhouse, Lever, Ashby, Himalayas, Jobicy, RemoteOK, and Arbeitnow job discovery; resume-based job matching and scheduled email/SMS notifications. Review project details and metrics before using in an application."

func SetupRoutes(r *gin.Engine, db *gorm.DB) {
	r.POST("/internal/scheduler/run", GitHubActionsAuth(), func(c *gin.Context) { RunScheduledRefresh(c, db) })
	r.POST("/api/internal/scheduler/run", GitHubActionsAuth(), func(c *gin.Context) { RunScheduledRefresh(c, db) })
	r.POST("/api/public/touch-activity", func(c *gin.Context) { TouchUserActivity(c, db) })

	api := r.Group("/api")
	api.Use(FirebaseAuth())

	// Sessions & Auth
	api.POST("/auth/session", func(c *gin.Context) { CreateSession(c, db) })

	// Users
	api.POST("/users/register", func(c *gin.Context) { RegisterUser(c, db) })
	api.POST("/users/resume", func(c *gin.Context) { UploadResume(c, db) })
	api.POST("/users/resume/scrape", func(c *gin.Context) { ScrapeResume(c, db) })
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
	api.PATCH("/drafts/:id", func(c *gin.Context) { UpdateDraftContent(c, db) })
	api.POST("/drafts/:id/status", func(c *gin.Context) { UpdateDraftStatus(c, db) })
	api.GET("/jobs/matches", func(c *gin.Context) { GetMatchedJobs(c, db) })
	api.POST("/jobs/matches/:id/draft", func(c *gin.Context) { PrepareJobDraft(c, db) })
	api.POST("/jobs/matches/:id/status", func(c *gin.Context) { UpdateMatchStatus(c, db) })
	api.GET("/jobs/export.xlsx", func(c *gin.Context) { ExportMatchedJobsXLSX(c, db) })
}

func resolveUserID(db *gorm.DB, userIDValue, firebaseUID, firebaseEmail string) (uint, error) {
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
	// 1. Check if user exists by firebase_uid
	if err := db.Where("firebase_uid = ?", firebaseUID).First(&user).Error; err == nil {
		now := time.Now()
		if user.LastActiveAt == nil || now.Sub(*user.LastActiveAt) > 30*time.Minute {
			db.Model(&user).Update("last_active_at", now)
		}
		cleanEmail := strings.TrimSpace(firebaseEmail)
		if user.Email == "" && cleanEmail != "" {
			db.Model(&user).Update("email", cleanEmail)
		}
		return user.ID, nil
	}

	// 2. If not found by firebase_uid, check if user exists by email
	cleanEmail := strings.TrimSpace(firebaseEmail)
	if cleanEmail != "" {
		if err := db.Where("LOWER(email) = LOWER(?)", cleanEmail).First(&user).Error; err == nil {
			now := time.Now()
			db.Model(&user).Updates(map[string]any{
				"firebase_uid":   firebaseUID,
				"last_active_at": now,
			})
			return user.ID, nil
		}
	}

	// 3. Just-In-Time (JIT) Auto-Provision user record so authenticated Firebase users never fail
	now := time.Now()
	targetEmail := cleanEmail
	if targetEmail == "" {
		targetEmail = firebaseUID + "@jobhunter.local"
	}
	user = repository.User{
		Email:        targetEmail,
		FirebaseUID:  firebaseUID,
		LastActiveAt: &now,
	}
	if err := db.Create(&user).Error; err != nil {
		if cleanEmail != "" {
			if err2 := db.Where("LOWER(email) = LOWER(?)", cleanEmail).First(&user).Error; err2 == nil {
				db.Model(&user).Updates(map[string]any{
					"firebase_uid":   firebaseUID,
					"last_active_at": now,
				})
				return user.ID, nil
			}
		}
		return 0, fmt.Errorf("failed to auto-provision user record: %v", err)
	}

	return user.ID, nil
}

func authenticatedUserID(c *gin.Context, db *gorm.DB) (uint, error) {
	// 1. Fast path: if user_id was verified from the 2-Hour Session JWT, return directly in 0.05ms with 0 DB queries!
	if val, exists := c.Get("user_id"); exists {
		if id, ok := val.(uint); ok && id > 0 {
			return id, nil
		}
	}

	// 2. Slow path: resolve by Firebase UID / Email and JIT-provision
	uid := c.GetString("firebase_uid")
	if uid == "" {
		return 0, fmt.Errorf("authenticated user session is required")
	}
	email := c.GetString("firebase_email")
	return resolveUserID(db, "", uid, email)
}

// CreateSession exchanges the user's initial authentication into a signed 2-Hour Session JWT.
func CreateSession(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var user repository.User
	if err := db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User record not found"})
		return
	}

	sessionToken, expiresAt, err := GenerateSessionToken(user.ID, user.Email, user.FirebaseUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate 2-hour session token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_token": sessionToken,
		"expires_at":    expiresAt,
		"user_id":       user.ID,
		"email":         user.Email,
		"message":       "2-hour session token created",
	})
}

type RegisterRequest struct {
	Email       string `json:"email" binding:"required"`
	FirebaseUID string `json:"firebase_uid" binding:"required"`
}

type TouchActivityRequest struct {
	Email string `json:"email" binding:"required"`
}

// TouchUserActivity records user activity (e.g., clicking 'Email me 2hr link') to reset their 5-day active window.
func TouchUserActivity(c *gin.Context, db *gorm.DB) {
	var req TouchActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}
	cleanEmail := strings.TrimSpace(req.Email)
	if cleanEmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email cannot be empty"})
		return
	}
	now := time.Now()
	res := db.Model(&repository.User{}).Where("LOWER(email) = LOWER(?)", cleanEmail).Update("last_active_at", now)
	if res.RowsAffected == 0 {
		var existing repository.User
		if err := db.Where("LOWER(email) = LOWER(?)", cleanEmail).First(&existing).Error; err != nil {
			newUser := repository.User{
				Email:        cleanEmail,
				FirebaseUID:  "pending-" + cleanEmail,
				LastActiveAt: &now,
			}
			db.Create(&newUser)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type ScrapeResumeRequest struct {
	ResumeText string `json:"resume_text" binding:"required"`
}

// ScrapeResume parses raw resume text via AI & local heuristics, saving extracted profile fields to the user record.
func ScrapeResume(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req ScrapeResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume_text is required"})
		return
	}

	text := strings.TrimSpace(req.ResumeText)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "resume_text cannot be empty"})
		return
	}

	var aiApplicantProfile services.ApplicantExtraction
	var extractionWarning string
	if generatedProfile, err := services.ExtractApplicantProfile(text); err == nil {
		aiApplicantProfile = generatedProfile
	} else {
		extractionWarning = "AI extraction notice: " + err.Error()
	}

	updates := map[string]any{
		"resume_text": text,
	}
	applicantValues := applicantExtractionValues(aiApplicantProfile)
	for column, value := range extractApplicantDetailsLocal(text) {
		if applicantValues[column] == "" {
			applicantValues[column] = value
		}
	}
	for column, value := range applicantValues {
		if strings.TrimSpace(value) != "" {
			updates[column] = strings.TrimSpace(value)
		}
	}
	for column, value := range map[string]string{
		"profile_summary": aiApplicantProfile.ProfileSummary,
		"target_roles":    aiApplicantProfile.TargetRoles,
		"skills":          aiApplicantProfile.Skills,
		"location":        aiApplicantProfile.Location,
	} {
		if strings.TrimSpace(value) != "" {
			updates[column] = strings.TrimSpace(value)
		}
	}

	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update applicant profile"})
		return
	}

	var updatedUser repository.User
	db.First(&updatedUser, userID)

	c.JSON(http.StatusOK, gin.H{
		"message":           "Resume parsed and profile updated successfully",
		"warning":           extractionWarning,
		"applicant_profile": updatedUser,
		"profile_summary":   updatedUser.ProfileSummary,
	})
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
	now := time.Now()
	user := repository.User{
		Email:        req.Email,
		FirebaseUID:  c.GetString("firebase_uid"),
		LastActiveAt: &now,
	}

	if err := db.Where("firebase_uid = ?", req.FirebaseUID).FirstOrCreate(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create user"})
		return
	}
	db.Model(&user).Update("last_active_at", now)

	sessionToken, expiresAt, _ := GenerateSessionToken(user.ID, user.Email, user.FirebaseUID)
	c.JSON(http.StatusOK, gin.H{
		"user":          user,
		"session_token": sessionToken,
		"expires_at":    expiresAt,
	})
}

func UploadResume(c *gin.Context, db *gorm.DB) {
	// Allow multipart framing overhead while keeping the actual resume file capped below.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 13<<20)
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

	filePath := filepath.Join(uploadDir, fmt.Sprintf("%d-%d%s", userID, time.Now().UnixNano(), ext))

	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	resumeText, extractionWarning := extractResumeText(filePath, ext)
	if strings.TrimSpace(resumeText) == "" {
		_ = os.Remove(filePath)
		message := extractionWarning
		if message == "" {
			message = "No readable text was found in this resume."
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": message})
		return
	}
	profileSummary := ""
	storagePath := strings.TrimSpace(c.PostForm("storage_path"))
	resumeURL := filePath
	if storagePath != "" {
		if !validFirebaseResumePath(storagePath, c.GetString("firebase_uid")) {
			_ = os.Remove(filePath)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Firebase Storage resume path"})
			return
		}
		resumeURL = "firebase-storage://" + storagePath
	}
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
		"resume_url":  resumeURL,
		"resume_text": resumeText,
	}
	// A newly uploaded resume is an explicit request to refresh resume-derived fields.
	// Preserve application preferences and answers (notice period, experience, CTC,
	// authorization), which are managed separately by the user in Profile settings.
	var existing repository.User
	if err := db.First(&existing, userID).Error; err != nil {
		_ = os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load applicant profile"})
		return
	}
	applicantValues := applicantExtractionValues(aiApplicantProfile)
	for column, value := range extractApplicantDetailsLocal(resumeText) {
		if applicantValues[column] == "" {
			applicantValues[column] = value
		}
	}
	for column, value := range applicantValues {
		if strings.TrimSpace(value) != "" {
			updates[column] = strings.TrimSpace(value)
		}
	}
	for column, value := range map[string]string{
		"profile_summary": aiApplicantProfile.ProfileSummary,
		"target_roles":    aiApplicantProfile.TargetRoles,
		"skills":          aiApplicantProfile.Skills,
		"location":        aiApplicantProfile.Location,
	} {
		if strings.TrimSpace(value) != "" {
			updates[column] = strings.TrimSpace(value)
		}
	}
	if strings.TrimSpace(existing.ApplicantProjects) == "" {
		updates["applicant_projects"] = jobHunterProjectHighlight
	}

	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		_ = os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resume path"})
		return
	}
	if existing.ResumeURL != "" && existing.ResumeURL != resumeURL && strings.HasPrefix(filepath.Clean(existing.ResumeURL), filepath.Clean(uploadDir)+string(os.PathSeparator)) {
		_ = os.Remove(existing.ResumeURL)
	}
	if storagePath != "" {
		_ = os.Remove(filePath)
	}

	var updatedUser repository.User
	_ = db.First(&updatedUser, userID).Error
	c.JSON(http.StatusOK, gin.H{
		"message":           "Resume uploaded successfully",
		"path":              filePath,
		"storage_path":      storagePath,
		"storage_used":      storagePath != "",
		"extracted_chars":   len(resumeText),
		"profile_summary":   profileSummary,
		"applicant_profile": profileResponse(updatedUser),
		"warning":           extractionWarning,
	})
}

func validFirebaseResumePath(path, firebaseUID string) bool {
	parts := strings.Split(path, "/")
	if firebaseUID == "" || len(parts) != 3 || parts[0] != "resumes" || parts[1] != firebaseUID || parts[2] == "" || filepath.Base(parts[2]) != parts[2] {
		return false
	}
	switch strings.ToLower(filepath.Ext(parts[2])) {
	case ".pdf", ".docx", ".txt", ".md":
		return true
	default:
		return false
	}
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
	Provider       string    `json:"Provider"`
	SourceName     string    `json:"SourceName"`
	JobActive      bool      `json:"JobActive"`
	LastSeenAt     time.Time `json:"LastSeenAt"`
}

type MatchedJobView struct {
	ID             uint       `json:"ID"`
	JobID          uint       `json:"JobID"`
	JobTitle       string     `json:"JobTitle"`
	JobURL         string     `json:"JobURL"`
	ApplyURL       string     `json:"ApplyURL"`
	Location       string     `json:"Location"`
	PublishedAt    *time.Time `json:"PublishedAt"`
	LastSeenAt     time.Time  `json:"LastSeenAt"`
	JobActive      bool       `json:"JobActive"`
	Company        string     `json:"Company"`
	Provider       string     `json:"Provider"`
	SourceName     string     `json:"SourceName"`
	SourceURL      string     `json:"SourceURL"`
	MatchScore     int        `json:"MatchScore"`
	MatchReason    string     `json:"MatchReason"`
	MatchedSkills  string     `json:"MatchedSkills"`
	RequiredSkills string     `json:"RequiredSkills"`
	Status         string     `json:"Status"`
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
			Company:        job.Company,
			Provider:       job.Provider,
			SourceName:     source.Name,
			JobActive:      job.IsActive,
			LastSeenAt:     job.LastSeenAt,
		})
	}
	c.JSON(http.StatusOK, views)
}

// PrepareJobDraft builds a fact-checked, editable application pack for one
// active profile match. Submission always happens on the employer's site.
func PrepareJobDraft(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	matchID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || matchID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid matched job ID"})
		return
	}
	var match repository.JobMatch
	if err := db.Where("id = ? AND user_id = ?", matchID, userID).First(&match).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matched job not found"})
		return
	}
	var job repository.Job
	if err := db.First(&job, match.JobID).Error; err != nil || !job.IsActive || (job.ApplyURL == "" && job.URL == "") {
		c.JSON(http.StatusConflict, gin.H{"error": "This listing is no longer confirmed active or has no employer application link"})
		return
	}
	var user repository.User
	if err := db.First(&user, userID).Error; err != nil || strings.TrimSpace(user.ResumeText) == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "Upload a readable resume before preparing application materials"})
		return
	}
	profileFacts := fmt.Sprintf("\n\nUser-confirmed application answers (use only when the form asks):\nNotice period: %s\nExperience: %s\nExpected compensation: %s\nApplicant project notes: %s",
		user.NoticePeriod, user.ApplicantYearsExperience, user.SalaryExpectation, user.ApplicantProjects)
	resumeForDraft := user.ResumeText + profileFacts
	jobContext := fmt.Sprintf("Title: %s\nCompany: %s\nLocation: %s\n\nDescription:\n%s", job.Title, job.Company, job.Location, job.Description)
	cover, coverErr := services.GenerateDraft(resumeForDraft, jobContext)
	if coverErr != nil {
		cover = buildApplicationMessage(user, job)
	}
	tailored, resumeErr := services.GenerateTailoredResume(user.ResumeText, job.Title, job.Description)
	if resumeErr != nil {
		tailored = services.BuildRoleFocusedResume(user, job.Title, job.Description)
	}

	var draft repository.JobDraft
	err = db.Where("user_id = ? AND job_id = ?", userID, job.ID).First(&draft).Error
	if err == gorm.ErrRecordNotFound {
		draft = repository.JobDraft{UserID: userID, JobID: job.ID, Status: "pending"}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load existing application pack"})
		return
	}
	draft.DraftContent = cover
	draft.TailoredResume = tailored
	draft.Status = "pending"
	if err := db.Save(&draft).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save application pack"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"draft_id": draft.ID, "message": "Application pack prepared for your review", "ai_cover_letter": coverErr == nil, "ai_tailored_resume": resumeErr == nil, "review_required": true})
}

func buildApplicationMessage(user repository.User, job repository.Job) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Application note — %s at %s\n\n", job.Title, job.Company)
	out.WriteString("I am interested in this role and would welcome the opportunity to discuss how my background aligns with the position. The attached resume contains my verified experience and skills.\n\n")
	if user.ApplicantFirstName != "" {
		out.WriteString("Regards,\n" + user.ApplicantFirstName + " " + user.ApplicantLastName)
	}
	return out.String()
}

func GetMatchedJobs(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var matches []repository.JobMatch
	if err := db.Where("user_id = ?", userID).Joins("JOIN jobs ON jobs.id = job_matches.job_id").Order("CASE WHEN jobs.provider = 'greenhouse' THEN 0 WHEN jobs.provider IN ('lever', 'ashby') THEN 1 WHEN jobs.provider = 'jobicy' THEN 2 WHEN jobs.provider = 'remoteok' THEN 3 WHEN jobs.provider = 'arbeitnow' THEN 4 WHEN jobs.provider = 'himalayas' THEN 5 ELSE 6 END, job_matches.match_score DESC, job_matches.created_at DESC").Find(&matches).Error; err != nil {
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
		views = append(views, MatchedJobView{
			ID:             match.ID,
			JobID:          match.JobID,
			JobTitle:       job.Title,
			JobURL:         job.URL,
			ApplyURL:       job.ApplyURL,
			Location:       job.Location,
			PublishedAt:    job.PublishedAt,
			LastSeenAt:     job.LastSeenAt,
			JobActive:      job.IsActive,
			Company:        job.Company,
			Provider:       job.Provider,
			SourceName:     source.Name,
			SourceURL:      source.URL,
			MatchScore:     match.MatchScore,
			MatchReason:    match.MatchReason,
			MatchedSkills:  match.MatchedSkills,
			RequiredSkills: match.RequiredSkills,
			Status:         match.Status,
		})
	}
	c.JSON(http.StatusOK, views)
}

func UpdateMatchStatus(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != "applied" && req.Status != "not_applied" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be applied or not_applied"})
		return
	}
	var match repository.JobMatch
	if err := db.Where("id = ? AND user_id = ?", c.Param("id"), userID).First(&match).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Matched job not found"})
		return
	}
	if req.Status == "applied" {
		var job repository.Job
		if err := db.First(&job, match.JobID).Error; err != nil || !job.IsActive || (job.ApplyURL == "" && job.URL == "") {
			c.JSON(http.StatusConflict, gin.H{"error": "This listing is no longer confirmed active. Verify it on the employer site before marking applied."})
			return
		}
	}
	if err := db.Model(&match).Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update application status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": req.Status, "message": "Application status saved as user-reported"})
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
		UserID                   uint   `json:"user_id"`
		FirebaseUID              string `json:"firebase_uid"`
		ProfileSummary           string `json:"profile_summary"`
		TargetRoles              string `json:"target_roles"`
		Skills                   string `json:"skills"`
		Location                 string `json:"location"`
		CronFrequency            string `json:"cron_frequency"`
		ApplicantFirstName       string `json:"applicant_first_name"`
		ApplicantLastName        string `json:"applicant_last_name"`
		ApplicantPhone           string `json:"applicant_phone"`
		ApplicantAddress         string `json:"applicant_address"`
		ApplicantCity            string `json:"applicant_city"`
		ApplicantRegion          string `json:"applicant_region"`
		ApplicantPostalCode      string `json:"applicant_postal_code"`
		ApplicantCountry         string `json:"applicant_country"`
		ApplicantLinkedIn        string `json:"applicant_linkedin"`
		ApplicantWebsite         string `json:"applicant_website"`
		ApplicantGitHub          string `json:"applicant_github"`
		ApplicantProjects        string `json:"applicant_projects"`
		WorkAuthorization        string `json:"work_authorization"`
		VisaSponsorship          string `json:"visa_sponsorship"`
		NoticePeriod             string `json:"notice_period"`
		ApplicantYearsExperience string `json:"applicant_years_experience"`
		SalaryExpectation        string `json:"salary_expectation"`
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
		"profile_summary":            req.ProfileSummary,
		"target_roles":               req.TargetRoles,
		"skills":                     req.Skills,
		"location":                   req.Location,
		"applicant_first_name":       req.ApplicantFirstName,
		"applicant_last_name":        req.ApplicantLastName,
		"applicant_phone":            req.ApplicantPhone,
		"applicant_address":          req.ApplicantAddress,
		"applicant_city":             req.ApplicantCity,
		"applicant_region":           req.ApplicantRegion,
		"applicant_postal_code":      req.ApplicantPostalCode,
		"applicant_country":          req.ApplicantCountry,
		"applicant_linkedin":         req.ApplicantLinkedIn,
		"applicant_website":          req.ApplicantWebsite,
		"applicant_github":           req.ApplicantGitHub,
		"applicant_projects":         req.ApplicantProjects,
		"work_authorization":         req.WorkAuthorization,
		"visa_sponsorship":           req.VisaSponsorship,
		"notice_period":              req.NoticePeriod,
		"applicant_years_experience": req.ApplicantYearsExperience,
		"salary_expectation":         req.SalaryExpectation,
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
	// Seed the explicit answers the user requested once; later profile edits win.
	answerDefaults := map[string]any{}
	if strings.TrimSpace(user.NoticePeriod) == "" {
		user.NoticePeriod = "Immediate joiner"
		answerDefaults["notice_period"] = user.NoticePeriod
	}
	if strings.TrimSpace(user.ApplicantYearsExperience) == "" {
		user.ApplicantYearsExperience = "2.2 years"
		answerDefaults["applicant_years_experience"] = user.ApplicantYearsExperience
	}
	if strings.TrimSpace(user.SalaryExpectation) == "" {
		user.SalaryExpectation = "₹10 LPA"
		answerDefaults["salary_expectation"] = user.SalaryExpectation
	}
	if len(answerDefaults) > 0 {
		db.Model(&repository.User{}).Where("id = ?", userID).Updates(answerDefaults)
	}
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
	db.Where("user_id = ?", userID).Joins("JOIN jobs ON jobs.id = job_matches.job_id").Order("CASE WHEN jobs.provider = 'greenhouse' THEN 0 WHEN jobs.provider IN ('lever', 'ashby') THEN 1 WHEN jobs.provider = 'jobicy' THEN 2 WHEN jobs.provider = 'remoteok' THEN 3 WHEN jobs.provider = 'arbeitnow' THEN 4 WHEN jobs.provider = 'himalayas' THEN 5 ELSE 6 END, job_matches.match_score DESC, job_matches.created_at DESC").Limit(20).Find(&matches)
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
		matchedJobs = append(matchedJobs, MatchedJobView{
			ID:             match.ID,
			JobID:          match.JobID,
			JobTitle:       job.Title,
			JobURL:         job.URL,
			ApplyURL:       job.ApplyURL,
			Location:       job.Location,
			PublishedAt:    job.PublishedAt,
			LastSeenAt:     job.LastSeenAt,
			JobActive:      job.IsActive,
			Company:        job.Company,
			Provider:       job.Provider,
			SourceName:     source.Name,
			SourceURL:      source.URL,
			MatchScore:     match.MatchScore,
			MatchReason:    match.MatchReason,
			MatchedSkills:  match.MatchedSkills,
			RequiredSkills: match.RequiredSkills,
			Status:         match.Status,
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
		"id":                         user.ID,
		"email":                      user.Email,
		"resume_url":                 user.ResumeURL,
		"resume_text":                user.ResumeText,
		"profile_summary":            user.ProfileSummary,
		"target_roles":               user.TargetRoles,
		"skills":                     user.Skills,
		"location":                   user.Location,
		"applicant_first_name":       user.ApplicantFirstName,
		"applicant_last_name":        user.ApplicantLastName,
		"applicant_phone":            user.ApplicantPhone,
		"applicant_address":          user.ApplicantAddress,
		"applicant_city":             user.ApplicantCity,
		"applicant_region":           user.ApplicantRegion,
		"applicant_postal_code":      user.ApplicantPostalCode,
		"applicant_country":          user.ApplicantCountry,
		"applicant_linkedin":         user.ApplicantLinkedIn,
		"applicant_website":          user.ApplicantWebsite,
		"applicant_github":           user.ApplicantGitHub,
		"applicant_projects":         user.ApplicantProjects,
		"work_authorization":         user.WorkAuthorization,
		"visa_sponsorship":           user.VisaSponsorship,
		"notice_period":              user.NoticePeriod,
		"applicant_years_experience": user.ApplicantYearsExperience,
		"salary_expectation":         user.SalaryExpectation,
		"cron_frequency":             user.CronFrequency,
		"whatsapp_number":            user.WhatsAppNumber,
		"phone_verified":             user.PhoneVerified,
		"sms_opt_in":                 user.SMSOptIn,
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


// RunScheduledRefresh is called by the private GitHub Actions cron after source refresh.
func RunScheduledRefresh(c *gin.Context, db *gorm.DB) {
	scheduler.RunScrapingCycle(db)

	type RecipientInfo struct {
		Email       string `json:"email"`
		Skills      string `json:"skills"`
		TargetRoles string `json:"target_roles"`
		Location    string `json:"location"`
	}
	var users []repository.User
	var recipients []RecipientInfo
	// Only deliver job digests to users active in the past 5 days (or newly registered within 5 days)
	cutoff := time.Now().Add(-5 * 24 * time.Hour)
	if err := db.Where("email != '' AND (last_active_at >= ? OR (last_active_at IS NULL AND created_at >= ?))", cutoff, cutoff).Find(&users).Error; err == nil {
		for _, u := range users {
			cleanEmail := strings.TrimSpace(u.Email)
			if cleanEmail != "" {
				recipients = append(recipients, RecipientInfo{
					Email:       cleanEmail,
					Skills:      u.Skills,
					TargetRoles: u.TargetRoles,
					Location:    u.Location,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     scheduler.GetStatus().LastRunMessage,
		"last_run_at": scheduler.GetStatus().LastRunAt,
		"recipients":  recipients,
	})
}


func UpdateDraftContent(c *gin.Context, db *gorm.DB) {
	userID, err := authenticatedUserID(c, db)
	if err != nil { c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()}); return }
	var req struct { DraftContent string `json:"DraftContent"`; TailoredResume string `json:"TailoredResume"` }
	if err := c.ShouldBindJSON(&req); err != nil { c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}); return }
	if len(req.DraftContent) > 100000 || len(req.TailoredResume) > 100000 { c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Draft text must be 100,000 characters or less"}); return }
	result := db.Model(&repository.JobDraft{}).Where("id = ? AND user_id = ?", c.Param("id"), userID).Updates(map[string]any{"draft_content": req.DraftContent, "tailored_resume": req.TailoredResume})
	if result.Error != nil { c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save draft"}); return }
	if result.RowsAffected == 0 { c.JSON(http.StatusNotFound, gin.H{"error": "Draft not found"}); return }
	c.JSON(http.StatusOK, gin.H{"message": "Draft saved"})
}
