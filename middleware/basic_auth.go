package middleware

import (
	"net/http"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const platformBasicAuthRealm = `Basic realm="Platform"`

func PlatformBasicAuthEnabled() bool {
	return config.PlatformBasicAuthEnabled()
}

func RequirePlatformBasicAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !EnsurePlatformBasicAuth(c) {
			return
		}
		c.Next()
	}
}

func EnsurePlatformBasicAuth(c *gin.Context) bool {
	if !PlatformBasicAuthEnabled() {
		return true
	}

	username, password, ok := c.Request.BasicAuth()
	if ok {
		db := config.GetDB()
		var user models.User
		if err := db.Where("username = ?", username).First(&user).Error; err == nil {
			if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil {
				c.Set(gin.AuthUserKey, username)
				return true
			}
		}
	}

	c.Header("WWW-Authenticate", platformBasicAuthRealm)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
	return false
}
