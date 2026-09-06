package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
)

const (
	infoGatherMaxConcurrent = 3
	infoGatherDefaultTimeout = 180 * time.Second
)

var (
	infoGatherSem     = make(chan struct{}, infoGatherMaxConcurrent)
	infoGatherRunMu   sync.Mutex
	infoGatherRunning = map[uint]struct{}{}
)

type createInfoGatherJobRequest struct {
	Target         string `json:"target"`
	Notes          string `json:"notes"`
	IncludeXSearch bool   `json:"include_x_search"`
}

// GetInfoGatherJobs lists jobs for the current user (admins see all).
func GetInfoGatherJobs(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var jobs []models.InfoGatherJob
	if err := scopeByOwner(db.Model(&models.InfoGatherJob{}), user).
		Order("id DESC").
		Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list info gather jobs"})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

// GetInfoGatherJob returns one job.
func GetInfoGatherJob(c *gin.Context) {
	job, ok := loadInfoGatherJobOwned(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, job)
}

// GetInfoGatherFindings returns findings for a job.
func GetInfoGatherFindings(c *gin.Context) {
	job, ok := loadInfoGatherJobOwned(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var findings []models.InfoGatherFinding
	if err := db.Where("job_id = ?", job.ID).Order("id ASC").Find(&findings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list findings"})
		return
	}
	c.JSON(http.StatusOK, findings)
}

// CreateInfoGatherJob creates a job and starts async AI search.
func CreateInfoGatherJob(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	var req createInfoGatherJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target is required"})
		return
	}
	if len(target) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target is too long"})
		return
	}

	profile, apiKey, active := ActiveAIConfig()
	if !active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No AI model is enabled. Configure one in System → AI Settings"})
		return
	}
	if strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enabled AI profile has no API key"})
		return
	}

	job := models.InfoGatherJob{
		Target:         target,
		Notes:          strings.TrimSpace(req.Notes),
		IncludeXSearch: req.IncludeXSearch,
		Status:         models.InfoGatherJobPending,
		Model:          profile.Model,
		CreatedBy:      user.ID,
	}
	db := config.GetDB()
	if err := db.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job"})
		return
	}

	go runInfoGatherJob(job.ID)
	c.JSON(http.StatusCreated, job)
}

// RetryInfoGatherJob clears findings and re-runs the job.
func RetryInfoGatherJob(c *gin.Context) {
	job, ok := loadInfoGatherJobOwned(c)
	if !ok {
		return
	}
	if job.Status == models.InfoGatherJobRunning || job.Status == models.InfoGatherJobPending {
		c.JSON(http.StatusConflict, gin.H{"error": "Job is already running"})
		return
	}
	profile, apiKey, active := ActiveAIConfig()
	if !active || strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No AI model is enabled. Configure one in System → AI Settings"})
		return
	}

	db := config.GetDB()
	if err := db.Where("job_id = ?", job.ID).Delete(&models.InfoGatherFinding{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear previous findings"})
		return
	}
	now := time.Now()
	updates := map[string]any{
		"status":         models.InfoGatherJobPending,
		"error_message":  "",
		"raw_response":   "",
		"summary_notes":  "",
		"email_count":    0,
		"phone_count":    0,
		"finding_count":  0,
		"model":          profile.Model,
		"started_at":     nil,
		"finished_at":    nil,
		"updated_at":     now,
	}
	if err := db.Model(&job).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset job"})
		return
	}
	_ = db.First(&job, job.ID)
	go runInfoGatherJob(job.ID)
	c.JSON(http.StatusOK, job)
}

// DeleteInfoGatherJob deletes a job and its findings (owner or admin).
func DeleteInfoGatherJob(c *gin.Context) {
	job, ok := loadInfoGatherJobOwned(c)
	if !ok {
		return
	}
	if job.Status == models.InfoGatherJobRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot delete a running job"})
		return
	}
	db := config.GetDB()
	tx := db.Begin()
	if err := tx.Where("job_id = ?", job.ID).Delete(&models.InfoGatherFinding{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete findings"})
		return
	}
	if err := tx.Delete(&job).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete job"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit delete"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func loadInfoGatherJobOwned(c *gin.Context) (models.InfoGatherJob, bool) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return models.InfoGatherJob{}, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job id"})
		return models.InfoGatherJob{}, false
	}
	db := config.GetDB()
	var job models.InfoGatherJob
	if err := db.First(&job, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return models.InfoGatherJob{}, false
	}
	if !canAccessOwned(user, job.CreatedBy) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return models.InfoGatherJob{}, false
	}
	return job, true
}

func runInfoGatherJob(jobID uint) {
	infoGatherRunMu.Lock()
	if _, busy := infoGatherRunning[jobID]; busy {
		infoGatherRunMu.Unlock()
		return
	}
	infoGatherRunning[jobID] = struct{}{}
	infoGatherRunMu.Unlock()
	defer func() {
		infoGatherRunMu.Lock()
		delete(infoGatherRunning, jobID)
		infoGatherRunMu.Unlock()
	}()

	infoGatherSem <- struct{}{}
	defer func() { <-infoGatherSem }()

	db := config.GetDB()
	var job models.InfoGatherJob
	if err := db.First(&job, jobID).Error; err != nil {
		return
	}

	now := time.Now()
	_ = db.Model(&job).Updates(map[string]any{
		"status":     models.InfoGatherJobRunning,
		"started_at": now,
		"updated_at": now,
	}).Error

	profile, apiKey, active := ActiveAIConfig()
	if !active || strings.TrimSpace(apiKey) == "" {
		finishInfoGatherJob(jobID, models.InfoGatherJobFailed, "No AI model is enabled", "", nil, "")
		return
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = infoGatherDefaultTimeout
	}
	if timeout < 120*time.Second {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout+30*time.Second)
	defer cancel()

	result, err := utils.GatherContactsWithAI(ctx, profile.BaseURL, apiKey, profile.Model, timeout, utils.InfoGatherInput{
		Target:         job.Target,
		Notes:          job.Notes,
		IncludeXSearch: job.IncludeXSearch,
	})

	if err != nil && len(result.Findings) == 0 {
		finishInfoGatherJob(jobID, models.InfoGatherJobFailed, err.Error(), result.RawResponse, nil, result.Notes)
		return
	}

	status := models.InfoGatherJobSucceeded
	errMsg := ""
	if err != nil {
		status = models.InfoGatherJobPartial
		errMsg = err.Error()
	}

	findings := make([]models.InfoGatherFinding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, models.InfoGatherFinding{
			JobID:      jobID,
			Kind:       f.Kind,
			Value:      f.Value,
			Label:      f.Label,
			SourceURL:  f.SourceURL,
			Snippet:    f.Snippet,
			Confidence: f.Confidence,
		})
	}
	finishInfoGatherJob(jobID, status, errMsg, result.RawResponse, findings, result.Notes)
}

func finishInfoGatherJob(jobID uint, status models.InfoGatherJobStatus, errMsg, raw string, findings []models.InfoGatherFinding, notes string) {
	db := config.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return
	}

	emailCount, phoneCount := 0, 0
	if len(findings) > 0 {
		_ = tx.Where("job_id = ?", jobID).Delete(&models.InfoGatherFinding{})
		for i := range findings {
			findings[i].JobID = jobID
			if err := tx.Create(&findings[i]).Error; err != nil {
				// unique conflict → skip duplicate
				continue
			}
			switch findings[i].Kind {
			case "email":
				emailCount++
			case "phone":
				phoneCount++
			}
		}
	} else {
		var existing []models.InfoGatherFinding
		_ = tx.Where("job_id = ?", jobID).Find(&existing).Error
		for _, f := range existing {
			switch f.Kind {
			case "email":
				emailCount++
			case "phone":
				phoneCount++
			}
		}
		findings = existing
	}

	finished := time.Now()
	updates := map[string]any{
		"status":         status,
		"error_message":  errMsg,
		"raw_response":   raw,
		"summary_notes":  notes,
		"email_count":    emailCount,
		"phone_count":    phoneCount,
		"finding_count":  len(findings),
		"finished_at":    finished,
		"updated_at":     finished,
	}
	if err := tx.Model(&models.InfoGatherJob{}).Where("id = ?", jobID).Updates(updates).Error; err != nil {
		tx.Rollback()
		return
	}
	_ = tx.Commit()
}
