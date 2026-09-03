package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

// JWTSecretBytes returns the signing secret for optional token inspection
// (e.g. logout audit when AuthMiddleware is not on the route).
func JWTSecretBytes() []byte {
	return jwtSecret
}

func InitJWT() {
	secret := config.JWTSecret()
	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			panic(fmt.Errorf("failed to generate jwt secret: %w", err))
		}
		secret = base64.StdEncoding.EncodeToString(buf)
	}
	jwtSecret = []byte(secret)
}

// Claims represents the JWT claims structure
type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}

// AuthMiddleware validates JWT tokens
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(*Claims); ok && token.Valid {
			// Get user from database
			var user models.User
			db := config.GetDB()
			if err := db.First(&user, claims.UserID).Error; err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
				c.Abort()
				return
			}

			user.Role = models.NormalizeRole(user.Role)
			// Store user in context
			c.Set("user", user)
			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}
	}
}

// RequireAdmin restricts a route to admin-role users.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := c.Get("user")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		user, ok := raw.(models.User)
		if !ok || !models.NormalizeRole(user.Role).IsAdmin() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// CurrentUser returns the authenticated user from context, if present.
func CurrentUser(c *gin.Context) (models.User, bool) {
	raw, ok := c.Get("user")
	if !ok {
		return models.User{}, false
	}
	user, ok := raw.(models.User)
	if !ok {
		return models.User{}, false
	}
	user.Role = models.NormalizeRole(user.Role)
	return user, true
}

// GenerateToken generates a JWT token for a user (valid for 4 hours)
func GenerateToken(user models.User) (string, error) {
	expirationTime := jwt.NewNumericDate(time.Now().Add(4 * time.Hour))

	claims := &Claims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: expirationTime,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GenerateRefreshToken generates a refresh token (valid for 7 days)
func GenerateRefreshToken(user models.User) (string, error) {
	expirationTime := jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour))

	claims := &Claims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: expirationTime,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
