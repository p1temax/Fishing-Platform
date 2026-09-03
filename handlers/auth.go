package handlers

import (
	"net/http"
	"strings"

	"fishing-platform-backend/audit"
	"fishing-platform-backend/config"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// RegisterRequest represents the registration request
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginRequest represents the login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UserResponse represents the user response (without password)
type UserResponse struct {
	ID       uint            `json:"id"`
	Username string          `json:"username"`
	Role     models.UserRole `json:"role"`
}

// Register handles user registration
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	db := config.GetDB()

	// Check if user already exists
	var existingUser models.User
	if err := db.Where("username = ?", req.Username).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username already exists"})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// Create user
	user := models.User{
		Username: req.Username,
		Password: string(hashedPassword),
	}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Generate tokens
	accessToken, err := middleware.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	refreshToken, err := middleware.GenerateRefreshToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user": UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     models.NormalizeRole(user.Role),
		},
		"access":  accessToken,
		"refresh": refreshToken,
	})
}

// Login handles user login
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please provide username and password"})
		return
	}

	db := config.GetDB()
	username := strings.TrimSpace(req.Username)

	recordLogin := func(action string, actorID *uint, actorName string, status int, summary string) {
		audit.RecordAudit(db, audit.AuditEntry{
			ActorID:      actorID,
			ActorName:    actorName,
			Action:       action,
			ResourceType: "auth",
			Method:       http.MethodPost,
			Path:         c.Request.URL.Path,
			StatusCode:   status,
			Summary:      summary,
			ClientIP:     c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
		})
	}

	// Find user
	var user models.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		recordLogin("login_failed", nil, username, http.StatusUnauthorized, username+" login failed")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Username or password incorrect"})
		return
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		recordLogin("login_failed", nil, username, http.StatusUnauthorized, username+" login failed")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Username or password incorrect"})
		return
	}

	// Generate tokens
	accessToken, err := middleware.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	refreshToken, err := middleware.GenerateRefreshToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	actorID := user.ID
	recordLogin("login_success", &actorID, user.Username, http.StatusOK, user.Username+" login success")

	c.JSON(http.StatusOK, gin.H{
		"user": UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Role:     models.NormalizeRole(user.Role),
		},
		"access":  accessToken,
		"refresh": refreshToken,
	})
}

// Logout handles user logout (client-side token removal)
func Logout(c *gin.Context) {
	actorID, actorName := optionalActorFromAuthHeader(c)
	summary := "logout"
	if actorName != "" {
		summary = actorName + " logout"
	}
	audit.RecordAudit(config.GetDB(), audit.AuditEntry{
		ActorID:      actorID,
		ActorName:    actorName,
		Action:       "logout",
		ResourceType: "auth",
		Method:       http.MethodPost,
		Path:         c.Request.URL.Path,
		StatusCode:   http.StatusOK,
		Summary:      summary,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func optionalActorFromAuthHeader(c *gin.Context) (*uint, string) {
	authHeader := c.GetHeader("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return nil, ""
	}

	token, err := jwt.ParseWithClaims(parts[1], &middleware.Claims{}, func(token *jwt.Token) (interface{}, error) {
		return middleware.JWTSecretBytes(), nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil, ""
	}
	claims, ok := token.Claims.(*middleware.Claims)
	if !ok {
		return nil, ""
	}

	var user models.User
	if err := config.GetDB().First(&user, claims.UserID).Error; err != nil {
		id := claims.UserID
		return &id, ""
	}
	id := user.ID
	return &id, user.Username
}

// GetContainerToken provides a token for generated container services
func GetContainerToken(c *gin.Context) {
	var req struct {
		Secret string `json:"secret" form:"secret" binding:"required"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Secret is required"})
		return
	}

	containerSecret := config.ContainerSecret()
	if containerSecret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Container token exchange is not configured"})
		return
	}

	if req.Secret != containerSecret {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid secret"})
		return
	}

	db := config.GetDB()

	// Get admin user
	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin user not found"})
		return
	}

	// Generate token
	token, err := middleware.GenerateToken(admin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}
