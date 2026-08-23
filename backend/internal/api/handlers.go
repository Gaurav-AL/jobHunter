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

	"backend/internal/repository"
	"backend/internal/scheduler"
	"backend/internal/scraper"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRoutes(r *gin.Engine, db *gorm.DB) {
	api := r.Group("/api")

	// Users
	api.POST("/users/register", func(c *gin.Context) { RegisterUser(c, db) })
	api.POST("/users/resume", func(c *gin.Context) { UploadResume(c, db) })
	api.POST("/users/settings", func(c *gin.Context) { UpdateSettings(c, db) })
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

	user := repository.User{
		Email:       req.Email,
		FirebaseUID: req.FirebaseUID,
	}

	if err := db.Where("firebase_uid = ?", req.FirebaseUID).FirstOrCreate(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

func UploadResume(c *gin.Context, db *gorm.DB) {
	userID, err := resolveUserID(db, c.PostForm("user_id"), c.PostForm("firebase_uid"))
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
	if resumeText != "" {
		if generatedProfile, err := services.GenerateProfileSummary(resumeText); err == nil {
			profileSummary = generatedProfile
		} else {
			extractionWarning = strings.TrimSpace(extractionWarning + " AICC profile extraction skipped: " + err.Error())
		}
	}

	updates := map[string]any{
		"resume_url":  filePath,
		"resume_text": resumeText,
	}
	if profileSummary != "" {
		updates["profile_summary"] = profileSummary
	}

	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resume path"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Resume uploaded successfully",
		"path":            filePath,
		"extracted_chars": len(resumeText),
		"profile_summary": profileSummary,
		"warning":         extractionWarning,
	})
}

type SettingsRequest struct {
	UserID         uint   `json:"user_id"`
	FirebaseUID    string `json:"firebase_uid"`
	CronFrequency  string `json:"cron_frequency"`
	WhatsAppNumber string `json:"whatsapp_number"`
}

func UpdateSettings(c *gin.Context, db *gorm.DB) {
	var req SettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDValue := ""
	if req.UserID != 0 {
		userIDValue = strconv.FormatUint(uint64(req.UserID), 10)
	}
	userID, err := resolveUserID(db, userIDValue, req.FirebaseUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.Model(&repository.User{}).Where("id = ?", userID).Updates(repository.User{
		CronFrequency:  req.CronFrequency,
		WhatsAppNumber: req.WhatsAppNumber,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Settings updated"})
}

func GetSources(c *gin.Context, db *gorm.DB) {
	var sources []repository.AllowedSource
	firebaseUID := c.Query("firebase_uid")
	if firebaseUID != "" {
		userID, err := resolveUserID(db, c.Query("user_id"), firebaseUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := db.Where("is_global = ? OR user_id = ?", true, userID).Order("created_at DESC").Find(&sources).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sources"})
			return
		}
		c.JSON(http.StatusOK, sources)
		return
	}

	if err := db.Order("created_at DESC").Find(&sources).Error; err != nil {
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
	if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
		sourceURL = "https://" + sourceURL
	}
	sourceName := strings.TrimSpace(req.Name)
	if sourceName == "" {
		sourceName = strings.TrimSpace(req.NameUpper)
	}
	if sourceName == "" {
		sourceName = sourceURL
	}

	var userID *uint
	if !req.IsGlobal {
		userIDValue := ""
		if req.UserID != 0 {
			userIDValue = strconv.FormatUint(uint64(req.UserID), 10)
		}
		resolvedID, err := resolveUserID(db, userIDValue, req.FirebaseUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		userID = &resolvedID
	}

	source := repository.AllowedSource{
		URL:      sourceURL,
		Name:     sourceName,
		IsGlobal: req.IsGlobal,
		UserID:   userID,
	}
	if err := db.Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create source. It may already exist."})
		return
	}
	c.JSON(http.StatusOK, source)
}

type DraftView struct {
	ID           uint   `json:"ID"`
	JobID        uint   `json:"JobID"`
	DraftContent string `json:"DraftContent"`
	Status       string `json:"Status"`
	JobTitle     string `json:"JobTitle"`
	JobURL       string `json:"JobURL"`
	Company      string `json:"Company"`
}

type MatchedJobView struct {
	ID          uint   `json:"ID"`
	JobID       uint   `json:"JobID"`
	JobTitle    string `json:"JobTitle"`
	JobURL      string `json:"JobURL"`
	Company     string `json:"Company"`
	SourceURL   string `json:"SourceURL"`
	MatchScore  int    `json:"MatchScore"`
	MatchReason string `json:"MatchReason"`
	Status      string `json:"Status"`
}

func GetDrafts(c *gin.Context, db *gorm.DB) {
	userID, err := resolveUserID(db, c.Query("user_id"), c.Query("firebase_uid"))
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
			ID:           draft.ID,
			JobID:        draft.JobID,
			DraftContent: draft.DraftContent,
			Status:       draft.Status,
			JobTitle:     job.Title,
			JobURL:       job.URL,
			Company:      source.Name,
		})
	}
	c.JSON(http.StatusOK, views)
}

func GetMatchedJobs(c *gin.Context, db *gorm.DB) {
	userID, err := resolveUserID(db, c.Query("user_id"), c.Query("firebase_uid"))
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
		if job.SourceID != 0 {
			db.First(&source, job.SourceID)
		}
		views = append(views, MatchedJobView{
			ID:          match.ID,
			JobID:       match.JobID,
			JobTitle:    job.Title,
			JobURL:      job.URL,
			Company:     source.Name,
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

	if err := db.Model(&repository.JobDraft{}).Where("id = ?", draftID).Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
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
	userID, err := resolveUserID(db, c.Query("user_id"), c.Query("firebase_uid"))
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
		UserID         uint   `json:"user_id"`
		FirebaseUID    string `json:"firebase_uid"`
		ProfileSummary string `json:"profile_summary"`
		TargetRoles    string `json:"target_roles"`
		Skills         string `json:"skills"`
		Location       string `json:"location"`
		WhatsAppNumber string `json:"whatsapp_number"`
		CronFrequency  string `json:"cron_frequency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userIDValue := ""
	if req.UserID != 0 {
		userIDValue = strconv.FormatUint(uint64(req.UserID), 10)
	}
	userID, err := resolveUserID(db, userIDValue, req.FirebaseUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := repository.User{
		ProfileSummary: req.ProfileSummary,
		TargetRoles:    req.TargetRoles,
		Skills:         req.Skills,
		Location:       req.Location,
		WhatsAppNumber: req.WhatsAppNumber,
		CronFrequency:  req.CronFrequency,
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
	userID, err := resolveUserID(db, c.Query("user_id"), c.Query("firebase_uid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user repository.User
	db.First(&user, userID)
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
		if job.SourceID != 0 {
			db.First(&source, job.SourceID)
		}
		matchedJobs = append(matchedJobs, MatchedJobView{
			ID:          match.ID,
			JobID:       match.JobID,
			JobTitle:    job.Title,
			JobURL:      job.URL,
			Company:     source.Name,
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
		"id":              user.ID,
		"email":           user.Email,
		"resume_url":      user.ResumeURL,
		"resume_text":     user.ResumeText,
		"profile_summary": user.ProfileSummary,
		"target_roles":    user.TargetRoles,
		"skills":          user.Skills,
		"location":        user.Location,
		"cron_frequency":  user.CronFrequency,
		"whatsapp_number": user.WhatsAppNumber,
	}
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
