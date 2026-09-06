package models

import (
	"strings"
	"time"

	"fishing-platform-backend/utils"
	"gorm.io/gorm"
)

type RobotType string

const (
	RobotTypeFeishu   RobotType = "feishu"
	RobotTypeWecom    RobotType = "wecom"
	RobotTypeTelegram RobotType = "telegram"
	RobotTypeSlack    RobotType = "slack"
	RobotTypeDingTalk RobotType = "dingtalk"
	RobotTypeDiscord  RobotType = "discord"
)

type RobotStatus string

const (
	RobotStatusOnline  RobotStatus = "online"
	RobotStatusOffline RobotStatus = "offline"
)

type RobotPushStatus string

const (
	RobotPushStatusSuccess RobotPushStatus = "success"
	RobotPushStatusFailed  RobotPushStatus = "failed"
)

type Robot struct {
	ID          uint        `json:"id" gorm:"primaryKey"`
	Name        string      `json:"name" gorm:"size:100;not null"`
	Webhook     string      `json:"webhook" gorm:"not null"`
	Secret      string      `json:"secret" gorm:"size:500"` // Encrypted in database
	RobotType   RobotType   `json:"robot_type" gorm:"size:20;default:'feishu'"`
	Status      RobotStatus `json:"status" gorm:"size:20;default:'offline'"`
	Description string      `json:"description" gorm:"type:text"`
	CreatedBy   uint        `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	Projects    []Project   `json:"projects" gorm:"many2many:robot_projects;"`
}

type RobotPushLog struct {
	ID             uint            `json:"id" gorm:"primaryKey"`
	RobotID        uint            `json:"robot_id" gorm:"not null;index"`
	Robot          Robot           `json:"robot" gorm:"foreignKey:RobotID;constraint:OnDelete:CASCADE;"`
	RobotName      string          `json:"robot_name" gorm:"size:100"`
	RobotType      RobotType       `json:"robot_type" gorm:"size:20"`
	Trigger        string          `json:"trigger" gorm:"size:50;default:'test';index"`
	Status         RobotPushStatus `json:"status" gorm:"size:20;default:'failed';index"`
	Message        string          `json:"message" gorm:"type:text"`
	RequestPayload string          `json:"request_payload" gorm:"type:text"`
	ResponseStatus int             `json:"response_status"`
	ResponseBody   string          `json:"response_body" gorm:"type:text"`
	ErrorMessage   string          `json:"error_message" gorm:"type:text"`
	CreatedAt      time.Time       `json:"created_at" gorm:"index"`
}

// BeforeCreate hook to encrypt secret
func (r *Robot) BeforeCreate(tx *gorm.DB) error {
	if r.Secret != "" {
		encrypted, err := utils.Encrypt(r.Secret)
		if err != nil {
			return err
		}
		if encrypted != "" {
			r.Secret = encrypted
		}
	}
	return nil
}

// BeforeUpdate hook to encrypt secret
func (r *Robot) BeforeUpdate(tx *gorm.DB) error {
	if r.Secret != "" {
		// Check if it's already encrypted (base64 encoded)
		_, err := utils.Decrypt(r.Secret)
		if err != nil {
			// Not encrypted, encrypt it
			encrypted, err := utils.Encrypt(r.Secret)
			if err != nil {
				return err
			}
			if encrypted != "" {
				r.Secret = encrypted
			}
		}
	}
	return nil
}

// AfterFind hook to decrypt secret
func (r *Robot) AfterFind(tx *gorm.DB) error {
	if r.Secret != "" {
		decrypted, err := utils.Decrypt(r.Secret)
		if err == nil && decrypted != "" {
			r.Secret = decrypted
		}
	}
	return nil
}

type BuildStatus string

const (
	BuildStatusPending  BuildStatus = "pending"
	BuildStatusBuilding BuildStatus = "building"
	BuildStatusSuccess  BuildStatus = "success"
	BuildStatusFailed   BuildStatus = "failed"
)

type ContainerStatus string

const (
	ContainerStatusRunning  ContainerStatus = "running"
	ContainerStatusStopped  ContainerStatus = "stopped"
	ContainerStatusStopping ContainerStatus = "stopping"
	ContainerStatusStarting ContainerStatus = "starting"
)

type ProjectRunMode string

const (
	ProjectRunModeDocker ProjectRunMode = "docker"
	ProjectRunModeLocal  ProjectRunMode = "local"
)

type ProjectHealthStatus string

const (
	ProjectHealthStatusHealthy   ProjectHealthStatus = "healthy"
	ProjectHealthStatusUnhealthy ProjectHealthStatus = "unhealthy"
	ProjectHealthStatusStopped   ProjectHealthStatus = "stopped"
	ProjectHealthStatusUnknown   ProjectHealthStatus = "unknown"
)

type AgentStatus string

const (
	AgentStatusOnline  AgentStatus = "online"
	AgentStatusOffline AgentStatus = "offline"
)

type Agent struct {
	ID           uint        `json:"id" gorm:"primaryKey"`
	AgentID      string      `json:"agent_id" gorm:"size:100;not null;uniqueIndex"`
	Name         string      `json:"name" gorm:"size:100;not null"`
	TokenHash    string      `json:"-" gorm:"size:64;not null;uniqueIndex"`
	Hostname     string      `json:"hostname" gorm:"size:255"`
	IPAddress    string      `json:"ip_address" gorm:"size:64"`
	OS           string      `json:"os" gorm:"size:60"`
	Arch         string      `json:"arch" gorm:"size:60"`
	Version      string      `json:"version" gorm:"size:100"`
	Status       AgentStatus `json:"status" gorm:"size:20;default:'offline';index"`
	Capabilities string      `json:"capabilities" gorm:"type:text"`
	Tags         string      `json:"tags" gorm:"type:text"`
	LastSeenAt   *time.Time  `json:"last_seen_at" gorm:"index"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusDeploying DeploymentStatus = "deploying"
	DeploymentStatusRunning   DeploymentStatus = "running"
	DeploymentStatusStopped   DeploymentStatus = "stopped"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusRemoved   DeploymentStatus = "removed"
)

type AgentTaskStatus string

const (
	AgentTaskStatusPending   AgentTaskStatus = "pending"
	AgentTaskStatusLeased    AgentTaskStatus = "leased"
	AgentTaskStatusRunning   AgentTaskStatus = "running"
	AgentTaskStatusSucceeded AgentTaskStatus = "succeeded"
	AgentTaskStatusFailed    AgentTaskStatus = "failed"
	AgentTaskStatusCancelled AgentTaskStatus = "cancelled"
)

type Project struct {
	ID                 uint                `json:"id" gorm:"primaryKey"`
	Name               string              `json:"name" gorm:"size:100;not null"`
	Description        string              `json:"description" gorm:"type:text"`
	Robots             []Robot             `json:"robots" gorm:"many2many:robot_projects;"`
	RunMode            ProjectRunMode      `json:"run_mode" gorm:"size:20;default:'docker'"`
	FrontendRoute      string              `json:"frontend_route" gorm:"size:100;default:'/'"`
	ContainerRoute     string              `json:"container_route" gorm:"column:flask_route;size:100;default:'/api/submit'"`
	UseHTTPS           bool                `json:"use_https" gorm:"default:false"`
	ContainerCode      string              `json:"container_code" gorm:"column:flask_code;type:text"`
	ContainerStatus    ContainerStatus     `json:"container_status" gorm:"column:flask_status;size:20;default:'stopped'"`
	ContainerLog       string              `json:"container_log" gorm:"column:flask_log;type:text"`
	HtmlFilePath       string              `json:"html_file_path" gorm:"size:500"`
	SSLCertPath        string              `json:"ssl_cert_path" gorm:"size:500"`
	SSLKeyPath         string              `json:"ssl_key_path" gorm:"size:500"`
	DockerImage        string              `json:"docker_image" gorm:"size:200"`
	ContainerID        string              `json:"container_id" gorm:"size:255"`
	Port               uint                `json:"port" gorm:"default:6000"`
	LoginURL           string              `json:"login_url" gorm:"size:500"`
	BuildStatus        BuildStatus         `json:"build_status" gorm:"size:20;default:'pending'"`
	TaskID             string              `json:"task_id" gorm:"size:255"`
	DeploymentRevision int64               `json:"deployment_revision" gorm:"default:1"`
	DeletionPending    bool                `json:"deletion_pending" gorm:"default:false;index"`
	QrRelayID          *uint               `json:"qr_relay_id" gorm:"index"` // optional live QR relay for {{QR_RELAY_*}}
	CreatedBy          uint                `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	Credentials        []Credential        `json:"credentials" gorm:"foreignKey:ProjectID"`
	Messages           []Message           `json:"messages" gorm:"foreignKey:ProjectID"`
	Deployments        []ProjectDeployment `json:"deployments" gorm:"foreignKey:ProjectID"`
}

// ProjectDeployment is the isolated instance of one project on one agent.
// A project can have many deployments and an agent can host many projects.
type ProjectDeployment struct {
	ID               uint             `json:"id" gorm:"primaryKey"`
	ProjectID        uint             `json:"project_id" gorm:"not null;uniqueIndex:idx_project_agent"`
	Project          Project          `json:"-" gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE;"`
	AgentID          uint             `json:"agent_id" gorm:"not null;uniqueIndex:idx_project_agent;index"`
	Agent            Agent            `json:"agent" gorm:"foreignKey:AgentID;constraint:OnDelete:RESTRICT;"`
	DesiredStatus    DeploymentStatus `json:"desired_status" gorm:"size:20;default:'running'"`
	ActualStatus     DeploymentStatus `json:"actual_status" gorm:"size:20;default:'pending';index"`
	DesiredRevision  int64            `json:"desired_revision"`
	DeployedRevision int64            `json:"deployed_revision"`
	ContainerID      string           `json:"container_id" gorm:"size:255"`
	ContainerName    string           `json:"container_name" gorm:"size:255"`
	RuntimePort      uint             `json:"runtime_port"`
	RuntimeURL       string           `json:"runtime_url" gorm:"size:500"`
	LastError        string           `json:"last_error" gorm:"type:text"`
	LastSeenAt       *time.Time       `json:"last_seen_at"`
	LastSyncedAt     *time.Time       `json:"last_synced_at"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type AgentTask struct {
	ID             uint            `json:"id" gorm:"primaryKey"`
	TaskID         string          `json:"task_id" gorm:"size:100;not null;uniqueIndex"`
	AgentID        uint            `json:"agent_id" gorm:"not null;index"`
	ProjectID      uint            `json:"project_id" gorm:"not null;index"`
	DeploymentID   uint            `json:"deployment_id" gorm:"not null;index"`
	Type           string          `json:"type" gorm:"size:40;not null"`
	Payload        string          `json:"-" gorm:"type:text"`
	Status         AgentTaskStatus `json:"status" gorm:"size:20;default:'pending';index"`
	Attempt        int             `json:"attempt"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at" gorm:"index"`
	StartedAt      *time.Time      `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
	ErrorMessage   string          `json:"error_message" gorm:"type:text"`
	Result         string          `json:"result" gorm:"type:text"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ProjectLog is stored centrally and grouped by project while retaining its agent source.
type ProjectLog struct {
	ID           uint64    `json:"id" gorm:"primaryKey"`
	ProjectID    uint      `json:"project_id" gorm:"not null;index:idx_project_logs,priority:1"`
	AgentID      uint      `json:"agent_id" gorm:"not null;index"`
	DeploymentID uint      `json:"deployment_id" gorm:"not null;index;uniqueIndex:idx_deployment_sequence"`
	TaskID       string    `json:"task_id" gorm:"size:100;index"`
	Stream       string    `json:"stream" gorm:"size:20"`
	Level        string    `json:"level" gorm:"size:20;index"`
	Sequence     int64     `json:"sequence" gorm:"uniqueIndex:idx_deployment_sequence"`
	Message      string    `json:"message" gorm:"type:text"`
	LoggedAt     time.Time `json:"logged_at" gorm:"index:idx_project_logs,priority:2"`
	CreatedAt    time.Time `json:"created_at"`
}

type ProjectHealthCheck struct {
	ID              uint                `json:"id" gorm:"primaryKey"`
	ProjectID       uint                `json:"project_id" gorm:"not null;index"`
	Project         Project             `json:"project" gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE;"`
	AgentID         string              `json:"agent_id" gorm:"size:100;default:'local';index"`
	AgentName       string              `json:"agent_name" gorm:"size:100;default:'Local Host'"`
	RunMode         ProjectRunMode      `json:"run_mode" gorm:"size:20"`
	ContainerStatus ContainerStatus     `json:"container_status" gorm:"size:20"`
	RuntimeOK       bool                `json:"runtime_ok" gorm:"default:false"`
	Status          ProjectHealthStatus `json:"status" gorm:"size:20;default:'unknown';index"`
	URL             string              `json:"url" gorm:"size:500"`
	HTTPStatus      int                 `json:"http_status"`
	LatencyMS       int64               `json:"latency_ms"`
	Message         string              `json:"message" gorm:"type:text"`
	ErrorMessage    string              `json:"error_message" gorm:"type:text"`
	CheckedAt       time.Time           `json:"checked_at" gorm:"index"`
	CreatedAt       time.Time           `json:"created_at"`
}

type MessageStatus string

const (
	MessageStatusSuccess MessageStatus = "success"
	MessageStatusFailed  MessageStatus = "failed"
)

type Message struct {
	ID        uint          `json:"id" gorm:"primaryKey"`
	ProjectID uint          `json:"project_id" gorm:"not null;index"`
	Project   Project       `json:"project" gorm:"foreignKey:ProjectID"`
	Title     string        `json:"title" gorm:"size:200;not null"`
	Content   string        `json:"content" gorm:"type:text;not null"`
	Status    MessageStatus `json:"status" gorm:"size:20;default:'success'"`
	CreatedAt time.Time     `json:"created_at"`
}

type Credential struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	ProjectID    uint      `json:"project_id" gorm:"not null;index"`
	Project      Project   `json:"project" gorm:"foreignKey:ProjectID"`
	Username     string    `json:"username" gorm:"size:200"`
	Password     string    `json:"password" gorm:"size:500"` // Encrypted in database
	IPAddress    string    `json:"ip_address" gorm:"size:64"`
	IPLocation   string    `json:"ip_location" gorm:"size:255"`
	CaptchaValue string    `json:"captchavalue" gorm:"size:200"`
	CampaignID   *uint     `json:"campaign_id" gorm:"index"`
	RecipientID  *uint     `json:"recipient_id" gorm:"index"`
	CreatedAt    time.Time `json:"created_at"`
}

// BeforeCreate hook to encrypt password
func (c *Credential) BeforeCreate(tx *gorm.DB) error {
	if c.Password != "" {
		encrypted, err := utils.Encrypt(c.Password)
		if err != nil {
			return err
		}
		c.Password = encrypted
	}
	return nil
}

// AfterFind hook to decrypt password
func (c *Credential) AfterFind(tx *gorm.DB) error {
	if c.Password != "" {
		decrypted, err := utils.Decrypt(c.Password)
		if err == nil && decrypted != "" {
			c.Password = decrypted
		}
	}
	return nil
}

// BeforeUpdate hook to encrypt password
func (c *Credential) BeforeUpdate(tx *gorm.DB) error {
	if c.Password != "" {
		// Check if it's already encrypted (base64 encoded)
		_, err := utils.Decrypt(c.Password)
		if err != nil {
			// Not encrypted, encrypt it
			encrypted, err := utils.Encrypt(c.Password)
			if err != nil {
				return err
			}
			if encrypted != "" {
				c.Password = encrypted
			}
		}
	}
	return nil
}

type PhishingPage struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:100;not null;uniqueIndex:idx_phishing_owner_name"`
	URL         string    `json:"url" gorm:"size:200;not null"`
	Description string    `json:"description" gorm:"type:text"`
	Html        string    `json:"html" gorm:"type:text;not null"`
	CreatedBy   uint      `json:"created_by" gorm:"not null;default:0;uniqueIndex:idx_phishing_owner_name;index"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MailTrackingSetting is the singleton platform-managed mail tracking config (id=1).
type MailTrackingSetting struct {
	ID                  uint      `json:"id" gorm:"primaryKey"`
	PublicBaseURL       string    `json:"public_base_url" gorm:"size:500"`
	Enabled             bool      `json:"enabled" gorm:"default:true"`
	Secret              string    `json:"-" gorm:"size:500"` // encrypted at rest
	AllowRedirectHosts  string    `json:"allow_redirect_hosts" gorm:"type:text"` // one host per line
	ClickPath           string    `json:"click_path" gorm:"size:200"`
	OpenPath            string    `json:"open_path" gorm:"size:200"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// HostMetricSample stores periodic platform-host resource snapshots for dashboard trends.
type HostMetricSample struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	CollectedAt        time.Time `json:"collected_at" gorm:"index;not null"`
	CPUUsagePercent    float64   `json:"cpu_usage_percent"`
	MemoryUsagePercent float64   `json:"memory_usage_percent"`
	DiskUsagePercent   float64   `json:"disk_usage_percent"`
	MemoryTotalMB      uint64    `json:"memory_total_mb"`
	MemoryUsedMB       uint64    `json:"memory_used_mb"`
	DiskTotalGB        float64   `json:"disk_total_gb"`
	DiskUsedGB         float64   `json:"disk_used_gb"`
}

type IPBlacklist struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	IPAddress string    `json:"ip_address" gorm:"size:64;not null;uniqueIndex"`
	Reason    string    `json:"reason" gorm:"type:text"`
	Enabled   bool      `json:"enabled" gorm:"default:true;index"`
	CreatedBy uint      `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SmtpService stores reusable SMTP profiles for custom outbound mail.
type SmtpService struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:100;not null"`
	Host        string    `json:"host" gorm:"size:255;not null"`
	Port        int       `json:"port" gorm:"not null;default:587"`
	Username    string    `json:"username" gorm:"size:255"`
	Password    string    `json:"-" gorm:"size:500"` // Encrypted in database; never returned in JSON
	FromEmail   string    `json:"from_email" gorm:"size:255;not null"`
	FromName    string    `json:"from_name" gorm:"size:100"`
	UseTLS      bool      `json:"use_tls"`
	UseSSL      bool      `json:"use_ssl"`
	Enabled     bool      `json:"enabled" gorm:"index"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedBy   uint      `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *SmtpService) BeforeCreate(tx *gorm.DB) error {
	if s.Password == "" {
		return nil
	}
	encrypted, err := utils.Encrypt(s.Password)
	if err != nil {
		return err
	}
	if encrypted != "" {
		s.Password = encrypted
	}
	return nil
}

func (s *SmtpService) BeforeUpdate(tx *gorm.DB) error {
	if s.Password == "" {
		return nil
	}
	if _, err := utils.Decrypt(s.Password); err == nil {
		return nil
	}
	encrypted, err := utils.Encrypt(s.Password)
	if err != nil {
		return err
	}
	if encrypted != "" {
		s.Password = encrypted
	}
	return nil
}

func (s *SmtpService) AfterFind(tx *gorm.DB) error {
	if s.Password == "" {
		return nil
	}
	decrypted, err := utils.Decrypt(s.Password)
	if err == nil && decrypted != "" {
		s.Password = decrypted
	}
	return nil
}

// SmtpServicePublic is the API-safe representation (no password plaintext).
type SmtpServicePublic struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	HasPassword bool      `json:"has_password"`
	FromEmail   string    `json:"from_email"`
	FromName    string    `json:"from_name"`
	UseTLS      bool      `json:"use_tls"`
	UseSSL      bool      `json:"use_ssl"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s SmtpService) ToPublic() SmtpServicePublic {
	return SmtpServicePublic{
		ID:          s.ID,
		Name:        s.Name,
		Host:        s.Host,
		Port:        s.Port,
		Username:    s.Username,
		HasPassword: strings.TrimSpace(s.Password) != "",
		FromEmail:   s.FromEmail,
		FromName:    s.FromName,
		UseTLS:      s.UseTLS,
		UseSSL:      s.UseSSL,
		Enabled:     s.Enabled,
		Description: s.Description,
		CreatedBy:   s.CreatedBy,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}

type MailCampaignStatus string

const (
	MailCampaignStatusSending MailCampaignStatus = "sending"
	MailCampaignStatusSent    MailCampaignStatus = "sent"
	MailCampaignStatusPartial MailCampaignStatus = "partial"
	MailCampaignStatusFailed  MailCampaignStatus = "failed"
)

type MailEventKind string

const (
	MailEventOpen   MailEventKind = "open"
	MailEventClick  MailEventKind = "click"
	MailEventSubmit MailEventKind = "submit"
	MailEventSent   MailEventKind = "sent"
	MailEventError  MailEventKind = "error"
)

// MailCampaign is one workbench bulk-send activity.
type MailCampaign struct {
	ID             uint               `json:"id" gorm:"primaryKey"`
	SmtpServiceID  uint               `json:"smtp_service_id" gorm:"index;not null"`
	ProjectID      *uint              `json:"project_id" gorm:"index"`
	Subject        string             `json:"subject" gorm:"size:500;not null"`
	BodyTemplate   string             `json:"body_template" gorm:"type:text;not null"`
	IsHTML         bool               `json:"is_html" gorm:"default:false"`
	TrackOpens     bool               `json:"track_opens" gorm:"default:false"`
	TrackClicks    bool               `json:"track_clicks" gorm:"default:true"`
	LandingURL     string             `json:"landing_url" gorm:"size:1000"`
	ClickPath      string             `json:"click_path" gorm:"size:200;index"` // per-campaign opaque /api/... path
	OpenPath       string             `json:"open_path" gorm:"size:200;index"`  // per-campaign opaque /api/... path
	Status         MailCampaignStatus `json:"status" gorm:"size:20;default:'sending';index"`
	SentCount      int                `json:"sent_count" gorm:"default:0"`
	FailedCount    int                `json:"failed_count" gorm:"default:0"`
	OpenCount      int                `json:"open_count" gorm:"default:0"`
	ClickCount     int                `json:"click_count" gorm:"default:0"`
	SubmitCount    int                `json:"submit_count" gorm:"default:0"`
	RecipientCount int                `json:"recipient_count" gorm:"default:0"`
	CreatedBy      uint               `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// MailRecipient is one personalized recipient inside a campaign.
type MailRecipient struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	CampaignID   uint       `json:"campaign_id" gorm:"uniqueIndex:idx_mail_recipient_campaign_email;not null"`
	Email        string     `json:"email" gorm:"size:255;uniqueIndex:idx_mail_recipient_campaign_email;not null"`
	SentAt       *time.Time `json:"sent_at"`
	OpenedAt     *time.Time `json:"opened_at"`
	ClickedAt    *time.Time `json:"clicked_at"`
	SubmittedAt  *time.Time `json:"submitted_at"`
	OpenCount    int        `json:"open_count" gorm:"default:0"`
	ClickCount   int        `json:"click_count" gorm:"default:0"`
	SubmitCount  int        `json:"submit_count" gorm:"default:0"`
	LastOpenIP   string     `json:"last_open_ip" gorm:"size:64"`
	LastClickIP  string     `json:"last_click_ip" gorm:"size:64"`
	LastSubmitIP string     `json:"last_submit_ip" gorm:"size:64"`
	LastError    string     `json:"last_error" gorm:"type:text"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// MailEvent stores open/click/sent detail rows (no robot push).
type MailEvent struct {
	ID          uint          `json:"id" gorm:"primaryKey"`
	CampaignID  uint          `json:"campaign_id" gorm:"index;not null"`
	RecipientID uint          `json:"recipient_id" gorm:"index;not null"`
	Kind        MailEventKind `json:"kind" gorm:"size:20;index;not null"`
	IP          string        `json:"ip" gorm:"size:64"`
	UserAgent   string        `json:"user_agent" gorm:"size:500"`
	TargetURL   string        `json:"target_url" gorm:"size:1000"`
	CreatedAt   time.Time     `json:"created_at" gorm:"index"`
}

type UserRole string

const (
	UserRoleAdmin    UserRole = "admin"
	UserRoleOperator UserRole = "operator"
)

func (r UserRole) IsValid() bool {
	switch r {
	case UserRoleAdmin, UserRoleOperator:
		return true
	default:
		return false
	}
}

func (r UserRole) IsAdmin() bool {
	return r == UserRoleAdmin
}

// User model for authentication
type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"uniqueIndex;size:100;not null"`
	Password  string    `json:"-" gorm:"size:255;not null"` // Never return password in JSON
	Role      UserRole  `json:"role" gorm:"size:20;not null;default:'admin';index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NormalizeRole returns a valid role, defaulting empty/unknown values to admin
// for backward-compatible migration of existing accounts.
func NormalizeRole(role UserRole) UserRole {
	if role.IsValid() {
		return role
	}
	return UserRoleAdmin
}

// AIProfile stores an OpenAI-compatible model endpoint used by page mirroring.
// At most one profile should be Enabled at a time.
type AIProfile struct {
	ID         string    `json:"id" gorm:"primaryKey;size:64"`
	Name       string    `json:"name" gorm:"size:100;not null"`
	Enabled    bool      `json:"enabled" gorm:"index"`
	BaseURL    string    `json:"base_url" gorm:"size:500;not null"`
	APIKey     string    `json:"-" gorm:"size:500"` // Encrypted at rest; never return raw in list APIs
	Model      string    `json:"model" gorm:"size:100;not null"`
	TimeoutSec int       `json:"timeout_sec" gorm:"not null;default:120"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type InfoGatherJobStatus string

const (
	InfoGatherJobPending   InfoGatherJobStatus = "pending"
	InfoGatherJobRunning   InfoGatherJobStatus = "running"
	InfoGatherJobSucceeded InfoGatherJobStatus = "succeeded"
	InfoGatherJobFailed    InfoGatherJobStatus = "failed"
	InfoGatherJobPartial   InfoGatherJobStatus = "partial"
)

// InfoGatherJob is one AI web-search reconnaissance task.
type InfoGatherJob struct {
	ID             uint                `json:"id" gorm:"primaryKey"`
	Target         string              `json:"target" gorm:"size:500;not null"`
	Notes          string              `json:"notes" gorm:"type:text"`
	IncludeXSearch bool                `json:"include_x_search" gorm:"default:false"`
	Status         InfoGatherJobStatus `json:"status" gorm:"size:20;default:'pending';index"`
	ErrorMessage   string              `json:"error_message" gorm:"type:text"`
	RawResponse    string              `json:"-" gorm:"type:text"` // truncated model output; not listed in APIs
	SummaryNotes   string              `json:"summary_notes" gorm:"type:text"`
	EmailCount     int                 `json:"email_count" gorm:"default:0"`
	PhoneCount     int                 `json:"phone_count" gorm:"default:0"`
	FindingCount   int                 `json:"finding_count" gorm:"default:0"`
	Model          string              `json:"model" gorm:"size:100"`
	StartedAt      *time.Time          `json:"started_at"`
	FinishedAt     *time.Time          `json:"finished_at"`
	CreatedBy      uint                `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

// InfoGatherFinding is one structured contact/OSINT row from a job.
type InfoGatherFinding struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	JobID      uint      `json:"job_id" gorm:"index;not null;uniqueIndex:idx_info_gather_finding_dedupe"`
	Kind       string    `json:"kind" gorm:"size:20;not null;uniqueIndex:idx_info_gather_finding_dedupe"`
	Value      string    `json:"value" gorm:"size:500;not null;uniqueIndex:idx_info_gather_finding_dedupe"`
	Label      string    `json:"label" gorm:"size:255"`
	SourceURL  string    `json:"source_url" gorm:"size:1000"`
	Snippet    string    `json:"snippet" gorm:"type:text"`
	Confidence string    `json:"confidence" gorm:"size:20;default:'medium'"`
	CreatedAt  time.Time `json:"created_at"`
}

// QrRelay is a live QR-code relay channel: local capture uploads frames;
// a stable public URL always serves the latest image.
type QrRelay struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	Name         string     `json:"name" gorm:"size:100;not null"`
	Slug         string     `json:"slug" gorm:"size:64;not null;uniqueIndex"`
	TokenHash    string     `json:"-" gorm:"size:64;not null;index"`
	Enabled      bool       `json:"enabled" gorm:"default:true;index"`
	ImagePath    string     `json:"-" gorm:"size:500"`
	ContentType  string     `json:"content_type" gorm:"size:64"`
	ImageBytes   int64      `json:"image_bytes" gorm:"default:0"`
	Payload      string     `json:"payload" gorm:"type:text"` // optional decoded text from uploader
	PayloadHash  string     `json:"payload_hash" gorm:"size:64"`
	LastUploadAt *time.Time `json:"last_upload_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"` // upload or heartbeat
	UploadCount  int64      `json:"upload_count" gorm:"default:0"`
	CreatedBy    uint       `json:"created_by" gorm:"index;not null;default:0"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// QrRelayFrame is one historical uploaded frame for a relay (M3).
type QrRelayFrame struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	RelayID     uint      `json:"relay_id" gorm:"index;not null"`
	ImagePath   string    `json:"-" gorm:"size:500;not null"`
	ContentType string    `json:"content_type" gorm:"size:64"`
	ImageBytes  int64     `json:"image_bytes" gorm:"default:0"`
	Payload     string    `json:"payload" gorm:"type:text"`
	PayloadHash string    `json:"payload_hash" gorm:"size:64;index"`
	CreatedAt   time.Time `json:"created_at" gorm:"index"`
}

// AuditLog records immutable operator actions for compliance review.
// There is intentionally no UpdatedAt / soft-delete / delete API.
type AuditLog struct {
	ID           uint64    `json:"id" gorm:"primaryKey"`
	ActorID      *uint     `json:"actor_id" gorm:"index"`
	ActorName    string    `json:"actor_name" gorm:"size:100;index"`
	Action       string    `json:"action" gorm:"size:64;index"`
	ResourceType string    `json:"resource_type" gorm:"size:64;index"`
	ResourceID   string    `json:"resource_id" gorm:"size:64;index"`
	Method       string    `json:"method" gorm:"size:16"`
	Path         string    `json:"path" gorm:"size:255;index"`
	StatusCode   int       `json:"status_code" gorm:"index"`
	Success      bool      `json:"success" gorm:"index"`
	Summary      string    `json:"summary" gorm:"size:500"`
	Detail       string    `json:"detail" gorm:"type:text"`
	ClientIP     string    `json:"client_ip" gorm:"size:64;index"`
	UserAgent    string    `json:"user_agent" gorm:"size:255"`
	CreatedAt    time.Time `json:"created_at" gorm:"index"`
}

// dedupePhishingPages keeps the newest page per owner+name so a unique
// index can be applied safely after earlier racey uploads.
func dedupePhishingPages(db *gorm.DB) error {
	if !db.Migrator().HasTable(&PhishingPage{}) {
		return nil
	}
	return db.Exec(`
		DELETE FROM phishing_pages
		WHERE id NOT IN (
			SELECT MAX(id) FROM phishing_pages GROUP BY created_by, LOWER(name)
		)
	`).Error
}

// AutoMigrate runs database migrations
func AutoMigrate(db *gorm.DB) error {
	if err := dedupePhishingPages(db); err != nil {
		return err
	}

	err := db.AutoMigrate(
		&User{},
		&Robot{},
		&RobotPushLog{},
		&Agent{},
		&Project{},
		&ProjectDeployment{},
		&AgentTask{},
		&ProjectLog{},
		&ProjectHealthCheck{},
		&Message{},
		&Credential{},
		&PhishingPage{},
		&IPBlacklist{},
		&SmtpService{},
		&MailCampaign{},
		&MailRecipient{},
		&MailEvent{},
		&AuditLog{},
		&AIProfile{},
		&HostMetricSample{},
		&MailTrackingSetting{},
		&InfoGatherJob{},
		&InfoGatherFinding{},
		&QrRelay{},
		&QrRelayFrame{},
	)

	if err != nil {
		return err
	}

	// Backfill legacy users that predate the role column.
	if err := db.Model(&User{}).Where("role = '' OR role IS NULL").Update("role", UserRoleAdmin).Error; err != nil {
		return err
	}

	// Attribute legacy resources without an owner to the admin account.
	var admin User
	if err := db.Where("username = ? AND role = ?", "admin", UserRoleAdmin).First(&admin).Error; err != nil {
		if err := db.Where("role = ?", UserRoleAdmin).Order("id ASC").First(&admin).Error; err != nil {
			return nil
		}
	}
	if admin.ID == 0 {
		return nil
	}
	for _, model := range []any{&Project{}, &Robot{}, &IPBlacklist{}, &SmtpService{}, &MailCampaign{}, &PhishingPage{}, &InfoGatherJob{}, &QrRelay{}} {
		if err := db.Model(model).Where("created_by = 0 OR created_by IS NULL").Update("created_by", admin.ID).Error; err != nil {
			return err
		}
	}

	return nil
}
