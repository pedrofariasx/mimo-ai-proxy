/*
 * File: auth.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package middleware

import (
	"mimoproxy/internal/utils"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func ValidateApiKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := os.Getenv("API_KEY")
		if apiKey != "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") || strings.TrimPrefix(authHeader, "Bearer ") != apiKey {
				utils.SendError(c, http.StatusUnauthorized, "Invalid or missing API Key", "authentication_error", nil)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
