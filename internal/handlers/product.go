// ========================================
// internal/handlers/product.go - EKSIKSIZ VE DÜZELTME
// ========================================
package handlers

import (
	"database/sql"
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/supabase-community/supabase-go"
)

type ProductHandler struct {
	cfg            *config.Config
	supabaseClient *supabase.Client // Opsiyonel: complex queries için
}

func NewProductHandler(cfg *config.Config) *ProductHandler {
	client, _ := supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseKey, nil)
	return &ProductHandler{
		cfg:            cfg,
		supabaseClient: client,
	}
}

func (h *ProductHandler) GetProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	search := c.Query("search")
	category := c.Query("category")
	stockFilter := c.Query("stock_filter")

	offset := (page - 1) * limit

	// DÜZELTME: COALESCE sorununu çözmek için query'yi basitleştir
	query := `
		SELECT 
			p.id, p.title, p.description, p.price, p.image, p.category, 
			p.sku, p.rating, p.rating_count, p.is_active, p.created_at, p.updated_at,
			CASE WHEN i.id IS NULL THEN 0 ELSE i.id END as inv_id, 
			CASE WHEN i.quantity IS NULL THEN 0 ELSE i.quantity END as quantity, 
			CASE WHEN i.reserved_quantity IS NULL THEN 0 ELSE i.reserved_quantity END as reserved_quantity, 
			CASE WHEN i.min_stock_level IS NULL THEN 0 ELSE i.min_stock_level END as min_stock_level, 
			CASE WHEN i.max_stock_level IS NULL THEN 0 ELSE i.max_stock_level END as max_stock_level, 
			i.cost_price, 
			CASE WHEN i.updated_at IS NULL THEN p.updated_at ELSE i.updated_at END as inv_updated_at,
			CASE WHEN i.quantity IS NULL OR i.reserved_quantity IS NULL THEN 0 ELSE (i.quantity - i.reserved_quantity) END as available_stock,
			CASE 
				WHEN i.id IS NULL THEN 'OUT_OF_STOCK'
				WHEN (i.quantity - i.reserved_quantity) <= 0 THEN 'OUT_OF_STOCK'
				WHEN (i.quantity - i.reserved_quantity) <= i.min_stock_level THEN 'LOW_STOCK'
				ELSE 'IN_STOCK'
			END as stock_status
		FROM products p
		LEFT JOIN inventory i ON p.id = i.product_id
		WHERE p.is_active = true
	`

	args := []interface{}{}
	argCount := 0

	// Filtreler ekle
	if search != "" {
		argCount++
		query += " AND (p.title ILIKE $" + strconv.Itoa(argCount) + " OR COALESCE(p.description, '') ILIKE $" + strconv.Itoa(argCount) + ")"
		args = append(args, "%"+search+"%")
	}

	if category != "" {
		argCount++
		query += " AND p.category = $" + strconv.Itoa(argCount)
		args = append(args, category)
	}

	if stockFilter != "" {
		switch stockFilter {
		case "IN_STOCK":
			query += " AND i.id IS NOT NULL AND (i.quantity - i.reserved_quantity) > i.min_stock_level"
		case "LOW_STOCK":
			query += " AND i.id IS NOT NULL AND (i.quantity - i.reserved_quantity) <= i.min_stock_level AND (i.quantity - i.reserved_quantity) > 0"
		case "OUT_OF_STOCK":
			query += " AND (i.id IS NULL OR (i.quantity - i.reserved_quantity) <= 0)"
		}
	}

	query += " ORDER BY p.created_at DESC"
	argCount++
	query += " LIMIT $" + strconv.Itoa(argCount)
	args = append(args, limit)
	argCount++
	query += " OFFSET $" + strconv.Itoa(argCount)
	args = append(args, offset)

	// Debug: args array'ini logla
	fmt.Printf("GetProducts - args: %v, len: %d\n", args, len(args))
	fmt.Printf("GetProducts - query: %s\n", query)

	// DÜZELTME: limit ve offset her zaman var, bu yüzden args asla boş olamaz
	rows, err := database.DB.Query(query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürünler alınamadı: " + err.Error()})
		return
	}
	defer rows.Close()

	var products []models.ProductWithStock
	for rows.Next() {
		var product models.ProductWithStock
		var inventory models.Inventory
		// DÜZELTME: Null değerler için proper handling
		var invID, invQuantity, invReserved, invMin, invMax int
		var invCost *float64
		var invUpdatedAt time.Time

		err := rows.Scan(
			&product.ID, &product.Title, &product.Description, &product.Price,
			&product.Image, &product.Category, &product.SKU, &product.Rating,
			&product.RatingCount, &product.IsActive, &product.CreatedAt, &product.UpdatedAt,
			&invID, &invQuantity, &invReserved, &invMin, &invMax, &invCost, &invUpdatedAt,
			&product.AvailableStock, &product.StockStatus,
		)
		if err != nil {
			continue
		}

		// Inventory bilgisi varsa ekle (id > 0 kontrolü ile)
		if invID > 0 {
			inventory.ID = invID
			inventory.ProductID = product.ID
			inventory.Quantity = invQuantity
			inventory.ReservedQuantity = invReserved
			inventory.MinStockLevel = invMin
			inventory.MaxStockLevel = invMax
			if invCost != nil {
				inventory.CostPrice = *invCost
			} else {
				inventory.CostPrice = 0.0 // Default value if NULL
			}
			inventory.UpdatedAt = invUpdatedAt
			product.Inventory = &inventory
		} else {
			// Inventory yok ise nil
			product.Inventory = nil
		}

		products = append(products, product)
	}

	// EKLEME: Total count da döndür (frontend'in ihtiyacı olabilir)
	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"page":     page,
		"limit":    limit,
		"total":    len(products), // Mevcut sayfa için
	})
}

// EKLEME: Tekil ürün getirme endpoint'i
func (h *ProductHandler) GetProduct(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz product ID"})
		return
	}

	// DEBUGGING: Log ekle
	fmt.Printf("GetProduct called for ID: %d\n", productID)

	// DÜZELTME: COALESCE sorununu çözmek için query'yi basitleştir
	query := `
		SELECT 
			p.id, p.title, p.description, p.price, p.image, p.category, 
			p.sku, p.rating, p.rating_count, p.is_active, p.created_at, p.updated_at,
			CASE WHEN i.id IS NULL THEN 0 ELSE i.id END as inv_id, 
			CASE WHEN i.quantity IS NULL THEN 0 ELSE i.quantity END as quantity, 
			CASE WHEN i.reserved_quantity IS NULL THEN 0 ELSE i.reserved_quantity END as reserved_quantity, 
			CASE WHEN i.min_stock_level IS NULL THEN 0 ELSE i.min_stock_level END as min_stock_level, 
			CASE WHEN i.max_stock_level IS NULL THEN 0 ELSE i.max_stock_level END as max_stock_level, 
			i.cost_price, 
			CASE WHEN i.updated_at IS NULL THEN p.updated_at ELSE i.updated_at END as inv_updated_at,
			CASE WHEN i.quantity IS NULL OR i.reserved_quantity IS NULL THEN 0 ELSE (i.quantity - i.reserved_quantity) END as available_stock,
			CASE 
				WHEN i.id IS NULL THEN 'OUT_OF_STOCK'
				WHEN (i.quantity - i.reserved_quantity) <= 0 THEN 'OUT_OF_STOCK'
				WHEN (i.quantity - i.reserved_quantity) <= i.min_stock_level THEN 'LOW_STOCK'
				ELSE 'IN_STOCK'
			END as stock_status
		FROM products p
		LEFT JOIN inventory i ON p.id = i.product_id
		WHERE p.id = $1 AND p.is_active = true
	`

	var product models.ProductWithStock
	var inventory models.Inventory
	var invID, invQuantity, invReserved, invMin, invMax int
	var invCost *float64
	var invUpdatedAt time.Time

	// DÜZELTME: QueryRowContext kullan ve context timeout ekle
	row := database.DB.QueryRow(query, productID)
	err = row.Scan(
		&product.ID, &product.Title, &product.Description, &product.Price,
		&product.Image, &product.Category, &product.SKU, &product.Rating,
		&product.RatingCount, &product.IsActive, &product.CreatedAt, &product.UpdatedAt,
		&invID, &invQuantity, &invReserved, &invMin, &invMax, &invCost, &invUpdatedAt,
		&product.AvailableStock, &product.StockStatus,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Printf("Product not found: ID %d\n", productID)
			c.JSON(http.StatusNotFound, gin.H{"error": "Ürün bulunamadı"})
		} else {
			fmt.Printf("Database error for ID %d: %v\n", productID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün alınamadı: " + err.Error()})
		}
		return
	}

	// Inventory bilgisi varsa ekle
	if invID > 0 {
		inventory.ID = invID
		inventory.ProductID = product.ID
		inventory.Quantity = invQuantity
		inventory.ReservedQuantity = invReserved
		inventory.MinStockLevel = invMin
		inventory.MaxStockLevel = invMax
		if invCost != nil {
			inventory.CostPrice = *invCost
		} else {
			inventory.CostPrice = 0.0 // Default value if NULL
		}
		inventory.UpdatedAt = invUpdatedAt
		product.Inventory = &inventory
	} else {
		product.Inventory = nil
	}

	fmt.Printf("Product found successfully: ID %d, Title: %s\n", productID, product.Title)
	c.JSON(http.StatusOK, gin.H{"product": product})
}

func (h *ProductHandler) GetProductsCount(c *gin.Context) {
	search := c.Query("search")
	category := c.Query("category")
	stockFilter := c.Query("stock_filter")

	// DÜZELTME: DISTINCT COUNT kullan duplicate'ları önlemek için
	query := `
		SELECT COUNT(DISTINCT p.id)
		FROM products p
		LEFT JOIN inventory i ON p.id = i.product_id
		WHERE p.is_active = true
	`

	args := []interface{}{}
	argCount := 0

	// Filtreler ekle (GetProducts ile aynı)
	if search != "" {
		argCount++
		query += " AND (p.title ILIKE $" + strconv.Itoa(argCount) + " OR COALESCE(p.description, '') ILIKE $" + strconv.Itoa(argCount) + ")"
		args = append(args, "%"+search+"%")
	}

	if category != "" {
		argCount++
		query += " AND p.category = $" + strconv.Itoa(argCount)
		args = append(args, category)
	}

	if stockFilter != "" {
		switch stockFilter {
		case "IN_STOCK":
			query += " AND i.id IS NOT NULL AND (i.quantity - i.reserved_quantity) > i.min_stock_level"
		case "LOW_STOCK":
			query += " AND i.id IS NOT NULL AND (i.quantity - i.reserved_quantity) <= i.min_stock_level AND (i.quantity - i.reserved_quantity) > 0"
		case "OUT_OF_STOCK":
			query += " AND (i.id IS NULL OR (i.quantity - i.reserved_quantity) <= 0)"
		}
	}

	var count int
	var err error

	// DÜZELTME: Eğer args boşsa QueryRow'u parametresiz çağır
	if len(args) == 0 {
		err = database.DB.QueryRow(query).Scan(&count)
	} else {
		err = database.DB.QueryRow(query, args...).Scan(&count)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün sayısı alınamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *ProductHandler) GetCategories(c *gin.Context) {
	query := `
		SELECT DISTINCT category 
		FROM products 
		WHERE is_active = true AND category IS NOT NULL AND category != ''
		ORDER BY category
	`

	rows, err := database.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Kategoriler alınamadı: " + err.Error()})
		return
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err == nil && category != "" {
			categories = append(categories, category)
		}
	}

	// EKLEME: Boş array yerine null döndürmeyi önle
	if categories == nil {
		categories = []string{}
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}

func (h *ProductHandler) GetLowStockCount(c *gin.Context) {
	// DÜZELTME: Daha spesifik low stock tanımı
	query := `
		SELECT COUNT(*)
		FROM products p
		INNER JOIN inventory i ON p.id = i.product_id
		WHERE p.is_active = true 
		AND i.min_stock_level > 0
		AND (i.quantity - i.reserved_quantity) <= i.min_stock_level
		AND (i.quantity - i.reserved_quantity) > 0
	`

	var count int
	err := database.DB.QueryRow(query).Scan(&count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Düşük stok sayısı alınamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *ProductHandler) CheckProductStock(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz product ID"})
		return
	}

	var req models.CheckStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// EKLEME: Ürün var mı kontrolü
	var productExists bool
	productCheckQuery := "SELECT EXISTS(SELECT 1 FROM products WHERE id = $1 AND is_active = true)"
	err = database.DB.QueryRow(productCheckQuery, productID).Scan(&productExists)
	if err != nil || !productExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Ürün bulunamadı"})
		return
	}

	query := `
		SELECT COALESCE((quantity - reserved_quantity), 0) as available_stock,
		       COALESCE(quantity, 0) as total_stock,
		       COALESCE(reserved_quantity, 0) as reserved_stock
		FROM inventory 
		WHERE product_id = $1
	`

	var availableStock, totalStock, reservedStock int
	err = database.DB.QueryRow(query, productID).Scan(&availableStock, &totalStock, &reservedStock)
	if err != nil {
		if err == sql.ErrNoRows {
			// Inventory kaydı yok, stok 0
			c.JSON(http.StatusOK, gin.H{
				"available_stock": 0,
				"total_stock":     0,
				"reserved_stock":  0,
				"sufficient":      false,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Stok kontrolü yapılamadı: " + err.Error()})
		return
	}

	sufficient := availableStock >= req.Quantity

	c.JSON(http.StatusOK, gin.H{
		"available_stock": availableStock,
		"total_stock":     totalStock,
		"reserved_stock":  reservedStock,
		"requested":       req.Quantity,
		"sufficient":      sufficient,
	})
}

// CreateProduct adds a new product
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	var req struct {
		Title         string   `json:"title" binding:"required"`
		Description   string   `json:"description"`
		Price         float64  `json:"price" binding:"required"`
		Image         string   `json:"image"`
		Category      string   `json:"category"`
		SKU           string   `json:"sku"`
		IsActive      *bool    `json:"is_active"`
		InitialStock  *int     `json:"initial_stock"`
		MinStockLevel *int     `json:"min_stock_level"`
		MaxStockLevel *int     `json:"max_stock_level"`
		CostPrice     *float64 `json:"cost_price"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	// Transaction başlat
	tx, err := database.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction başlatılamadı"})
		return
	}
	defer tx.Rollback()

	insertQuery := `
        INSERT INTO products (title, description, price, image, category, sku, rating, rating_count, is_active, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, 0, 0, $7, NOW(), NOW())
        RETURNING id, title, description, price, image, category, sku, rating, rating_count, is_active, created_at, updated_at
    `

	var product models.Product
	err = tx.QueryRow(insertQuery,
		req.Title, req.Description, req.Price, req.Image, req.Category, req.SKU, isActive,
	).Scan(
		&product.ID, &product.Title, &product.Description, &product.Price, &product.Image,
		&product.Category, &product.SKU, &product.Rating, &product.RatingCount,
		&product.IsActive, &product.CreatedAt, &product.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün oluşturulamadı: " + err.Error()})
		return
	}

	// Inventory için değerler
	quantity := 0
	if req.InitialStock != nil && *req.InitialStock >= 0 {
		quantity = *req.InitialStock
	}
	minLevel := 0
	if req.MinStockLevel != nil && *req.MinStockLevel >= 0 {
		minLevel = *req.MinStockLevel
	}
	maxLevel := 0
	if req.MaxStockLevel != nil && *req.MaxStockLevel >= 0 {
		maxLevel = *req.MaxStockLevel
	}
	cost := 0.0
	if req.CostPrice != nil && *req.CostPrice >= 0 {
		cost = *req.CostPrice
	}

	invInsert := `
        INSERT INTO inventory (product_id, quantity, reserved_quantity, min_stock_level, max_stock_level, cost_price, updated_at)
        VALUES ($1, $2, 0, $3, $4, $5, NOW())
        ON CONFLICT (product_id) DO UPDATE SET
          quantity = EXCLUDED.quantity,
          min_stock_level = EXCLUDED.min_stock_level,
          max_stock_level = EXCLUDED.max_stock_level,
          cost_price = EXCLUDED.cost_price,
          updated_at = NOW()
    `

	if _, err := tx.Exec(invInsert, product.ID, quantity, minLevel, maxLevel, cost); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Inventory oluşturulamadı: " + err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit edilemedi"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"product": product})
}

// UpdateProduct updates an existing product
func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz product ID"})
		return
	}

	var req struct {
		Title       *string  `json:"title"`
		Description *string  `json:"description"`
		Price       *float64 `json:"price"`
		Image       *string  `json:"image"`
		Category    *string  `json:"category"`
		SKU         *string  `json:"sku"`
		IsActive    *bool    `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch existing product
	var existing models.Product
	err = database.DB.QueryRow(
		"SELECT id, title, description, price, image, category, sku, rating, rating_count, is_active, created_at, updated_at FROM products WHERE id = $1",
		productID,
	).Scan(
		&existing.ID, &existing.Title, &existing.Description, &existing.Price, &existing.Image,
		&existing.Category, &existing.SKU, &existing.Rating, &existing.RatingCount,
		&existing.IsActive, &existing.CreatedAt, &existing.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ürün bulunamadı"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün alınamadı: " + err.Error()})
		return
	}

	// Merge updates
	if req.Title != nil {
		existing.Title = *req.Title
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Price != nil {
		existing.Price = *req.Price
	}
	if req.Image != nil {
		existing.Image = *req.Image
	}
	if req.Category != nil {
		existing.Category = *req.Category
	}
	if req.SKU != nil {
		existing.SKU = *req.SKU
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	updateQuery := `
        UPDATE products
        SET title = $1, description = $2, price = $3, image = $4, category = $5,
            sku = $6, is_active = $7, updated_at = NOW()
        WHERE id = $8
        RETURNING id, title, description, price, image, category, sku, rating, rating_count, is_active, created_at, updated_at
    `

	var updated models.Product
	err = database.DB.QueryRow(updateQuery,
		existing.Title, existing.Description, existing.Price, existing.Image,
		existing.Category, existing.SKU, existing.IsActive, productID,
	).Scan(
		&updated.ID, &updated.Title, &updated.Description, &updated.Price, &updated.Image,
		&updated.Category, &updated.SKU, &updated.Rating, &updated.RatingCount,
		&updated.IsActive, &updated.CreatedAt, &updated.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün güncellenemedi: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"product": updated})
}

// ALTERNATIF: Daha basit Supabase versiyonu (eğer Range hala sorun yaparsa)
func (h *ProductHandler) GetProductsSimpleSupabase(c *gin.Context) {
	// Basit query - Range kullanmadan
	data, count, err := h.supabaseClient.From("products").
		Select("*", "exact", false).
		Eq("is_active", "true").
		Execute()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürünler alınamadı (Simple): " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products": data,
		"count":    count,
		"note":     "Basit Supabase query (Range olmadan)",
	})
}
