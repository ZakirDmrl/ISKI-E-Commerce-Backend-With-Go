// ========================================
// main.go - GÜNCELLENMIŞ VE EKSİKSİZ
// ========================================
package main

import (
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/handlers"
	"ecommerce-backend/internal/middleware"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	// Config yükle
	cfg := config.Load()

	// Database bağlantısı
	if err := database.Connect(cfg.DatabaseURL); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Gin mode set et
	if cfg.Port == "8080" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Gin router oluştur
	router := gin.Default()
	router.Static("/uploads", "./uploads")

	// Middleware'leri ekle
	router.Use(middleware.CORS())

	// Request logging middleware (development için)
	if gin.Mode() == gin.DebugMode {
		router.Use(gin.Logger())
	}

	// Recovery middleware
	router.Use(gin.Recovery())

	// Handlers oluştur
	authHandler := handlers.NewAuthHandler(cfg)
	productHandler := handlers.NewProductHandler(cfg)
	cartHandler := handlers.NewCartHandler(cfg)
	commentHandler := handlers.NewCommentHandler(cfg)
	profileHandler := handlers.NewProfileHandler(cfg)

	// API routes
	api := router.Group("/api/v1")
	{
		// Auth routes (public)
		auth := api.Group("/auth")
		{
			auth.POST("/signin", authHandler.SignIn)
			auth.POST("/signup", authHandler.SignUp)
			auth.POST("/signout", authHandler.SignOut)
			auth.POST("/refresh", authHandler.RefreshToken)
			auth.GET("/me", middleware.Auth(cfg.JWTSecret), authHandler.GetMe)
		}

		// Product routes (public) - GÜNCELLENMIŞ
		products := api.Group("/products")
		{
			// Ana product endpoints
			products.GET("", productHandler.GetProducts)            // /api/v1/products
			products.GET("/:id", productHandler.GetProduct)         // /api/v1/products/123
			products.GET("/count", productHandler.GetProductsCount) // /api/v1/products/count

			// Kategori ve stok endpoints
			products.GET("/categories", productHandler.GetCategories)         // /api/v1/products/categories
			products.GET("/low-stock-count", productHandler.GetLowStockCount) // /api/v1/products/low-stock-count

			// CRUD + Stok kontrol endpoint
			products.POST("", middleware.Auth(cfg.JWTSecret), productHandler.CreateProduct)    // POST /api/v1/products
			products.PUT("/:id", middleware.Auth(cfg.JWTSecret), productHandler.UpdateProduct) // PUT /api/v1/products/:id
			products.POST("/:id/check-stock", productHandler.CheckProductStock)                // /api/v1/products/123/check-stock
		}

		// Cart routes (protected)
		cart := api.Group("/cart").Use(middleware.Auth(cfg.JWTSecret))
		{
			cart.GET("", cartHandler.GetCartItems)                                 // GET /api/v1/cart
			cart.POST("/items", cartHandler.AddOrUpdateCartItem)                   // POST /api/v1/cart/items
			cart.PUT("/items/:productId/decrement", cartHandler.DecrementCartItem) // PUT /api/v1/cart/items/123/decrement
			cart.DELETE("/items/:productId", cartHandler.RemoveCartItem)           // DELETE /api/v1/cart/items/123
			cart.POST("/checkout", cartHandler.CreateOrder)                        // POST /api/v1/cart/checkout
		}

		// Comment routes (mixed access)
		comments := api.Group("/comments")
		{
			comments.GET("/product/:productId", commentHandler.GetComments)                                     // GET /api/v1/comments/product/123
			comments.POST("", middleware.Auth(cfg.JWTSecret), commentHandler.AddComment)                        // POST /api/v1/comments
			comments.POST("/:commentId/toggle-like", middleware.Auth(cfg.JWTSecret), commentHandler.ToggleLike) // POST /api/v1/comments/123/toggle-like
		}
		profile := api.Group("/profile").Use(middleware.Auth(cfg.JWTSecret))
		{
			profile.GET("", profileHandler.GetProfile)
			profile.PUT("", profileHandler.UpdateProfile)
			profile.POST("/avatar", profileHandler.UploadAvatar)

		}
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "OK",
			"message": "Server is running",
			"version": "1.0.0",
			"timestamp": gin.H{
				"server_time": gin.H{
					"utc": gin.H{
						"iso": gin.H{},
					},
				},
			},
		})
	})

	// API documentation endpoint (development için)
	if gin.Mode() == gin.DebugMode {
		router.GET("/api", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"api_version": "v1",
				"base_url":    "/api/v1",
				"endpoints": gin.H{
					"auth": gin.H{
						"POST /auth/signin":  "User login",
						"POST /auth/signup":  "User registration",
						"POST /auth/signout": "User logout",
						"POST /auth/refresh": "Refresh JWT token",
						"GET /auth/me":       "Get current user (protected)",
					},
					"products": gin.H{
						"GET /products":                  "Get products with pagination & filters",
						"GET /products/:id":              "Get single product by ID",
						"GET /products/count":            "Get total products count",
						"GET /products/categories":       "Get all categories",
						"GET /products/low-stock-count":  "Get low stock products count",
						"POST /products/:id/check-stock": "Check product stock availability",
						"GET /products/supabase":         "Get products via Supabase client (testing)",
					},
					"cart": gin.H{
						"GET /cart":                            "Get cart items (protected)",
						"POST /cart/items":                     "Add/update cart item (protected)",
						"PUT /cart/items/:productId/decrement": "Decrement cart item quantity (protected)",
						"DELETE /cart/items/:productId":        "Remove cart item (protected)",
						"POST /cart/checkout":                  "Create order from cart (protected)",
					},
					"comments": gin.H{
						"GET /comments/product/:productId":      "Get product comments",
						"POST /comments":                        "Add comment (protected)",
						"POST /comments/:commentId/toggle-like": "Toggle comment like (protected)",
					},
				},
			})
		})
	}

	// Catch-all route for undefined endpoints
	router.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{
			"error":   "Endpoint bulunamadı",
			"path":    c.Request.URL.Path,
			"method":  c.Request.Method,
			"message": "Bu endpoint mevcut değil. /api endpoint'ini ziyaret ederek mevcut API'leri görebilirsiniz.",
		})
	})

	// Server başlat
	log.Printf("🚀 Server starting on port %s", cfg.Port)
	log.Printf("📊 Database connected: %v", cfg.DatabaseURL != "")
	log.Printf("🔗 Supabase connected: %v", cfg.SupabaseURL != "")
	log.Printf("🌍 Environment: %s", gin.Mode())

	if gin.Mode() == gin.DebugMode {
		log.Printf("📖 API Documentation: http://localhost:%s/api", cfg.Port)
		log.Printf("❤️  Health Check: http://localhost:%s/health", cfg.Port)
	}

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
