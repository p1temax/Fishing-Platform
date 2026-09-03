package handlers

import (
	"net/http"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type userPublic struct {
	ID        uint            `json:"id"`
	Username  string          `json:"username"`
	Role      models.UserRole `json:"role"`
	CreatedAt interface{}     `json:"created_at"`
	UpdatedAt interface{}     `json:"updated_at"`
}

type createUserRequest struct {
	Username string          `json:"username" binding:"required"`
	Password string          `json:"password" binding:"required"`
	Role     models.UserRole `json:"role"`
}

type updateUserRequest struct {
	Role *models.UserRole `json:"role"`
}

func toUserPublic(user models.User) userPublic {
	return userPublic{
		ID:        user.ID,
		Username:  user.Username,
		Role:      models.NormalizeRole(user.Role),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

// GetCurrentUser returns the authenticated user profile.
func GetCurrentUser(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	c.JSON(http.StatusOK, toUserPublic(user))
}

// ListUsers lists all platform users (admin only).
func ListUsers(c *gin.Context) {
	db := config.GetDB()
	var users []models.User
	if err := db.Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users"})
		return
	}
	out := make([]userPublic, 0, len(users))
	for _, user := range users {
		out = append(out, toUserPublic(user))
	}
	c.JSON(http.StatusOK, out)
}

// CreateUser creates a user. Defaults to operator when role is omitted.
func CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username and password are required"})
		return
	}
	if len(password) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 6 characters"})
		return
	}

	role := models.UserRoleOperator
	if strings.TrimSpace(string(req.Role)) != "" {
		if !req.Role.IsValid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
			return
		}
		role = req.Role
	}

	db := config.GetDB()
	var existing models.User
	if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username already exists"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.User{
		Username: username,
		Password: string(hashed),
		Role:     role,
	}
	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, toUserPublic(user))
}

// UpdateUser updates role (admin only). Password resets use ResetUserPassword.
func UpdateUser(c *gin.Context) {
	db := config.GetDB()
	var user models.User
	if err := db.First(&user, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if req.Role == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}
	if !req.Role.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}
	newRole := *req.Role
	oldRole := models.NormalizeRole(user.Role)
	if oldRole.IsAdmin() && newRole == models.UserRoleOperator {
		if err := ensureAnotherAdmin(db, user.ID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	user.Role = newRole

	if err := db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, toUserPublic(user))
}

// ResetUserPassword generates a strong password for an operator account.
// The plaintext password is returned once in the response.
func ResetUserPassword(c *gin.Context) {
	db := config.GetDB()
	var user models.User
	if err := db.First(&user, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if models.NormalizeRole(user.Role).IsAdmin() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot reset password for admin accounts"})
		return
	}

	password := utils.GenerateSecurePassword(16)
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}
	user.Password = string(hashed)
	if err := db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":     toUserPublic(user),
		"password": password,
	})
}

// DeleteUser deletes a user (admin only).
func DeleteUser(c *gin.Context) {
	db := config.GetDB()
	var user models.User
	if err := db.First(&user, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	current, ok := middleware.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if current.ID == user.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete your own account"})
		return
	}
	if models.NormalizeRole(user.Role).IsAdmin() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete admin accounts"})
		return
	}

	if err := db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func ensureAnotherAdmin(db *gorm.DB, excludeID uint) error {
	var count int64
	if err := db.Model(&models.User{}).
		Where("role = ? AND id <> ?", models.UserRoleAdmin, excludeID).
		Count(&count).Error; err != nil {
		return err
	}
	if count < 1 {
		return errLastAdmin
	}
	return nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

const errLastAdmin simpleError = "Cannot remove the last admin user"
