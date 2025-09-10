// ========================================
// internal/middleware/cors.go - DÜZELTME
// ========================================
package middleware

import (
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Frontend URL'i environment'dan al, yoksa development için tüm origin'lere izin ver
		origin := "*"

		// Production için specific origin set edebilirsiniz
		// if gin.Mode() == gin.ReleaseMode {
		//     origin = "https://yourdomain.com"
		// }

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		// Preflight request için
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
}
