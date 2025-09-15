// ========================================
// internal/middleware/auth.go - DÜZELTME
// ========================================
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		fmt.Printf("Auth middleware - Authorization header: %s\n", authHeader)
		
		if authHeader == "" {
			fmt.Printf("Auth middleware - No authorization header\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// "Bearer " prefix'ini kaldır
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			fmt.Printf("Auth middleware - No Bearer prefix\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}
		
		fmt.Printf("Auth middleware - Token string: %s\n", tokenString)

		// Supabase JWT token'ı parse et
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Supabase JWT için signing method kontrolü
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			fmt.Printf("Auth middleware - Invalid token: %v\n", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Claims'leri al
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Supabase JWT claims yapısına uygun
			if sub, exists := claims["sub"]; exists {
				c.Set("userID", sub)
				fmt.Printf("Auth middleware - User ID set: %s\n", sub)
			}
			if email, exists := claims["email"]; exists {
				c.Set("userEmail", email)
				fmt.Printf("Auth middleware - User email set: %s\n", email)
			}
			// Role kontrolü eklenebilir
			if role, exists := claims["role"]; exists {
				c.Set("userRole", role)
				fmt.Printf("Auth middleware - User role set: %s\n", role)
			}
		}

		fmt.Printf("Auth middleware - Token validated successfully\n")
		c.Next()
	})
}
