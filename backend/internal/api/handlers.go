package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"backend/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRoutes(r *gin.Engine, db *gorm.DB) {
	api := r.Group("/api")

	// Users
	api.POST("/users/register", func(c *gin.Context) { RegisterUser(c, db) })
	api.POST("/users/resume", func(c *gin.Context) { UploadResume(c, db) })
	api.POST("/users/settings", func(c *gin.Context) { UpdateSettings(c, db) })

	// Sources
	api.GET("/sources", func(c *gin.Context) { GetSources(c, db) })
	api.POST("/sources", func(c *gin.Context) { AddSource(c, db) })

	// Drafts
	api.GET("/drafts", func(c *gin.Context) { GetDrafts(c, db) })
	api.POST("/drafts/:id/status", func(c *gin.Context) { UpdateDraftStatus(c, db) })
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
	// In a real app, verify the Firebase token to get the user ID
	userID := c.PostForm("user_id") // Mock user ID for now

	file, err := c.FormFile("resume")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resume file is required"})
		return
	}

	// Allowed formats: pdf, doc, docx, png, jpg, jpeg
	ext := filepath.Ext(file.Filename)
	// Just save it locally for now
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare upload directory"})
		return
	}

	filePath := fmt.Sprintf("%s/%s%s", uploadDir, userID, ext)

	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Update user record
	db.Model(&repository.User{}).Where("id = ?", userID).Update("resume_url", filePath)

	c.JSON(http.StatusOK, gin.H{"message": "Resume uploaded successfully", "path": filePath})
}

type SettingsRequest struct {
	UserID         uint   `json:"user_id" binding:"required"`
	CronFrequency  string `json:"cron_frequency"`
	WhatsAppNumber string `json:"whatsapp_number"`
}

func UpdateSettings(c *gin.Context, db *gorm.DB) {
	var req SettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := db.Model(&repository.User{}).Where("id = ?", req.UserID).Updates(repository.User{
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
	db.Find(&sources)
	c.JSON(http.StatusOK, sources)
}

func AddSource(c *gin.Context, db *gorm.DB) {
	var source repository.AllowedSource
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db.Create(&source)
	c.JSON(http.StatusOK, source)
}

func GetDrafts(c *gin.Context, db *gorm.DB) {
	userID := c.Query("user_id")
	var drafts []repository.JobDraft
	db.Where("user_id = ?", userID).Find(&drafts)
	c.JSON(http.StatusOK, drafts)
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

	db.Model(&repository.JobDraft{}).Where("id = ?", draftID).Update("status", req.Status)
	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}
