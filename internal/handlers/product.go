// ========================================
// internal/handlers/product.go - EKSIKSIZ VE DÜZELTME
// ========================================
package handlers

import (
	"database/sql"
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"
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

	// DÜZELTME: COALESCE kullanarak null değer güvenliği
	query := `
		SELECT 
			p.id, p.title, p.description, p.price, p.image, p.category, 
			p.sku, p.rating, p.rating_count, p.is_active, p.created_at, p.updated_at,
			COALESCE(i.id, 0) as inv_id, 
			COALESCE(i.quantity, 0) as quantity, 
			COALESCE(i.reserved_quantity, 0) as reserved_quantity, 
			COALESCE(i.min_stock_level, 0) as min_stock_level, 
			COALESCE(i.max_stock_level, 0) as max_stock_level, 
			i.cost_price, 
			COALESCE(i.updated_at, p.updated_at) as inv_updated_at,
			COALESCE((i.quantity - i.reserved_quantity), 0) as available_stock,
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

	query := `
		SELECT 
			p.id, p.title, p.description, p.price, p.image, p.category, 
			p.sku, p.rating, p.rating_count, p.is_active, p.created_at, p.updated_at,
			COALESCE(i.id, 0) as inv_id, 
			COALESCE(i.quantity, 0) as quantity, 
			COALESCE(i.reserved_quantity, 0) as reserved_quantity, 
			COALESCE(i.min_stock_level, 0) as min_stock_level, 
			COALESCE(i.max_stock_level, 0) as max_stock_level, 
			i.cost_price, 
			COALESCE(i.updated_at, p.updated_at) as inv_updated_at,
			COALESCE((i.quantity - i.reserved_quantity), 0) as available_stock,
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

	err = database.DB.QueryRow(query, productID).Scan(
		&product.ID, &product.Title, &product.Description, &product.Price,
		&product.Image, &product.Category, &product.SKU, &product.Rating,
		&product.RatingCount, &product.IsActive, &product.CreatedAt, &product.UpdatedAt,
		&invID, &invQuantity, &invReserved, &invMin, &invMax, &invCost, &invUpdatedAt,
		&product.AvailableStock, &product.StockStatus,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ürün bulunamadı"})
		} else {
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
	}

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
	err := database.DB.QueryRow(query, args...).Scan(&count)
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
