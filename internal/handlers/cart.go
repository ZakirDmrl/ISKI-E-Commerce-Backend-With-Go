// ========================================
// internal/handlers/cart.go - İYİLEŞTİRİLMİŞ VERSİYON
// ========================================
package handlers

import (
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/supabase-community/supabase-go"
)

type CartHandler struct {
	cfg            *config.Config
	supabaseClient *supabase.Client // Opsiyonel: database operasyonları için
}

func NewCartHandler(cfg *config.Config) *CartHandler {
	// Supabase client opsiyonel - sadece complex query'ler için kullanılabilir
	client, _ := supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseKey, nil)

	return &CartHandler{
		cfg:            cfg,
		supabaseClient: client,
	}
}

func (h *CartHandler) GetCartItems(c *gin.Context) {
	userID := c.GetString("userID")

	// DÜZELTME: created_at, updated_at eksikti, time.Time scan için düzeltme
	query := `
        SELECT 
            ci.id, ci.cart_id, ci.product_id, ci.quantity, ci.created_at,
            p.id, p.title, p.description, p.price, p.image, p.category, 
            p.sku, p.rating, p.rating_count, p.is_active, p.created_at, p.updated_at
        FROM cart_items ci
        JOIN carts ca ON ci.cart_id = ca.id
        JOIN products p ON ci.product_id = p.id
        WHERE ca.user_id = $1 AND p.is_active = true
        ORDER BY ci.created_at DESC
    `

	rows, err := database.DB.Query(query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sepet öğeleri alınamadı: " + err.Error()})
		return
	}
	defer rows.Close()

	var cartItems []models.CartItem
	for rows.Next() {
		var item models.CartItem
		var product models.Product

		err := rows.Scan(
			&item.ID, &item.CartID, &item.ProductID, &item.Quantity, &item.CreatedAt,
			&product.ID, &product.Title, &product.Description, &product.Price, &product.Image,
			&product.Category, &product.SKU, &product.Rating, &product.RatingCount,
			&product.IsActive, &product.CreatedAt, &product.UpdatedAt,
		)
		if err != nil {
			// Hata log'la ama devam et
			continue
		}

		item.Product = &product
		cartItems = append(cartItems, item)
	}

	c.JSON(http.StatusOK, gin.H{"items": cartItems})
}

func (h *CartHandler) AddOrUpdateCartItem(c *gin.Context) {
	userID := c.GetString("userID")

	var req models.AddCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// EKLEME: Ürün var mı kontrolü
	var productExists bool
	productCheckQuery := "SELECT EXISTS(SELECT 1 FROM products WHERE id = $1 AND is_active = true)"
	err := database.DB.QueryRow(productCheckQuery, req.ProductID).Scan(&productExists)
	if err != nil || !productExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Ürün bulunamadı"})
		return
	}

	// Stok kontrolü yap
	var availableStock int
	stockQuery := `
		SELECT COALESCE((i.quantity - i.reserved_quantity), 0) as available_stock
		FROM inventory i 
		WHERE i.product_id = $1
	`
	err = database.DB.QueryRow(stockQuery, req.ProductID).Scan(&availableStock)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Stok kontrolü yapılamadı: " + err.Error()})
		return
	}

	// EKLEME: Mevcut sepetteki miktarı da kontrol et
	var currentCartQuantity int
	currentCartQuery := `
		SELECT COALESCE(ci.quantity, 0)
		FROM carts c
		LEFT JOIN cart_items ci ON c.id = ci.cart_id AND ci.product_id = $2
		WHERE c.user_id = $1
	`
	database.DB.QueryRow(currentCartQuery, userID, req.ProductID).Scan(&currentCartQuantity)

	totalRequestedQuantity := currentCartQuantity + req.Quantity
	if availableStock < totalRequestedQuantity {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Yetersiz stok",
			"available": availableStock,
			"requested": totalRequestedQuantity,
			"in_cart":   currentCartQuantity,
			"adding":    req.Quantity,
		})
		return
	}

	// Transaction başlat
	tx, err := database.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction başlatılamadı"})
		return
	}
	defer tx.Rollback()

	// Kullanıcının cart'ını bul veya oluştur
	var cartID int
	cartQuery := "SELECT id FROM carts WHERE user_id = $1"
	err = tx.QueryRow(cartQuery, userID).Scan(&cartID)

	if err != nil {
		// Cart oluştur
		insertCartQuery := "INSERT INTO carts (user_id, created_at, updated_at) VALUES ($1, NOW(), NOW()) RETURNING id"
		err = tx.QueryRow(insertCartQuery, userID).Scan(&cartID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cart oluşturulamadı: " + err.Error()})
			return
		}
	}

	// Mevcut cart item'ı kontrol et
	var existingID int
	var existingQuantity int
	checkItemQuery := "SELECT id, quantity FROM cart_items WHERE cart_id = $1 AND product_id = $2"
	err = tx.QueryRow(checkItemQuery, cartID, req.ProductID).Scan(&existingID, &existingQuantity)

	var cartItem models.CartItem
	if err == nil {
		// Mevcut item'ı güncelle
		newQuantity := existingQuantity + req.Quantity
		updateQuery := `
			UPDATE cart_items 
			SET quantity = $1, updated_at = NOW() 
			WHERE id = $2 
			RETURNING id, cart_id, product_id, quantity, created_at
		`
		err = tx.QueryRow(updateQuery, newQuantity, existingID).Scan(
			&cartItem.ID, &cartItem.CartID, &cartItem.ProductID, &cartItem.Quantity, &cartItem.CreatedAt,
		)
	} else {
		// Yeni item ekle
		insertQuery := `
			INSERT INTO cart_items (cart_id, product_id, quantity, created_at) 
			VALUES ($1, $2, $3, NOW()) 
			RETURNING id, cart_id, product_id, quantity, created_at
		`
		err = tx.QueryRow(insertQuery, cartID, req.ProductID, req.Quantity).Scan(
			&cartItem.ID, &cartItem.CartID, &cartItem.ProductID, &cartItem.Quantity, &cartItem.CreatedAt,
		)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sepet güncellenemedi: " + err.Error()})
		return
	}

	// Transaction commit
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit edilemedi"})
		return
	}

	// Product bilgisini ekle
	var product models.Product
	productQuery := `
		SELECT id, title, description, price, image, category, sku, rating, rating_count, is_active, created_at, updated_at 
		FROM products WHERE id = $1
	`
	err = database.DB.QueryRow(productQuery, req.ProductID).Scan(
		&product.ID, &product.Title, &product.Description, &product.Price,
		&product.Image, &product.Category, &product.SKU, &product.Rating,
		&product.RatingCount, &product.IsActive, &product.CreatedAt, &product.UpdatedAt,
	)
	if err != nil {
		// Product bilgisi alamasa bile cart item'ı döndür
		cartItem.Product = nil
	} else {
		cartItem.Product = &product
	}

	c.JSON(http.StatusOK, cartItem)
}

func (h *CartHandler) DecrementCartItem(c *gin.Context) {
	userID := c.GetString("userID")
	productID, err := strconv.Atoi(c.Param("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz product ID"})
		return
	}

	// Transaction başlat
	tx, err := database.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction başlatılamadı"})
		return
	}
	defer tx.Rollback()

	// Kullanıcının cart'ını bul
	var cartID int
	err = tx.QueryRow("SELECT id FROM carts WHERE user_id = $1", userID).Scan(&cartID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sepet bulunamadı"})
		return
	}

	// Mevcut cart item'ı bul
	var existingID int
	var existingQuantity int
	err = tx.QueryRow(
		"SELECT id, quantity FROM cart_items WHERE cart_id = $1 AND product_id = $2",
		cartID, productID,
	).Scan(&existingID, &existingQuantity)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sepet öğesi bulunamadı"})
		return
	}

	var newQuantity int
	if existingQuantity <= 1 {
		// Miktar 1 ise tamamen kaldır
		_, err = tx.Exec("DELETE FROM cart_items WHERE id = $1", existingID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün kaldırılamadı: " + err.Error()})
			return
		}
		newQuantity = 0
	} else {
		// Miktarı azalt
		newQuantity = existingQuantity - 1
		_, err = tx.Exec(
			"UPDATE cart_items SET quantity = $1, updated_at = NOW() WHERE id = $2",
			newQuantity, existingID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Miktar azaltılamadı: " + err.Error()})
			return
		}
	}

	// Transaction commit
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit edilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"product_id":   productID,
		"new_quantity": newQuantity,
	})
}

func (h *CartHandler) RemoveCartItem(c *gin.Context) {
	userID := c.GetString("userID")
	productID, err := strconv.Atoi(c.Param("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz product ID"})
		return
	}

	// Transaction başlat
	tx, err := database.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction başlatılamadı"})
		return
	}
	defer tx.Rollback()

	// Kullanıcının cart'ını bul
	var cartID int
	err = tx.QueryRow("SELECT id FROM carts WHERE user_id = $1", userID).Scan(&cartID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sepet bulunamadı"})
		return
	}

	// Cart item'ı sil
	result, err := tx.Exec(
		"DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2",
		cartID, productID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün sepetten kaldırılamadı: " + err.Error()})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Ürün sepette bulunamadı"})
		return
	}

	// Transaction commit
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction commit edilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"product_id": productID})
}

func (h *CartHandler) CreateOrder(c *gin.Context) {
	userID := c.GetString("userID")

	var req models.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// EKLEME: Request validation
	if len(req.CartItems) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sepet boş"})
		return
	}

	if req.TotalAmount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz toplam tutar"})
		return
	}

	// Transaction başlat
	tx, err := database.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction başlatılamadı"})
		return
	}
	defer tx.Rollback()

	// EKLEME: Stok kontrolü sipariş öncesi
	for _, item := range req.CartItems {
		var availableStock int
		stockQuery := `
			SELECT COALESCE((quantity - reserved_quantity), 0) 
			FROM inventory 
			WHERE product_id = $1
		`
		err = tx.QueryRow(stockQuery, item.ProductID).Scan(&availableStock)
		if err != nil || availableStock < item.Quantity {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "Yetersiz stok",
				"product_id": item.ProductID,
				"available":  availableStock,
				"requested":  item.Quantity,
			})
			return
		}
	}

	// Sipariş oluştur
	var orderID int
	orderQuery := `
		INSERT INTO orders (user_id, total_amount, status, created_at, updated_at) 
		VALUES ($1, $2, 'pending', NOW(), NOW()) 
		RETURNING id
	`
	err = tx.QueryRow(orderQuery, userID, req.TotalAmount).Scan(&orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sipariş oluşturulamadı: " + err.Error()})
		return
	}

	// DÜZELTME: Sipariş öğelerini ekle ve fiyat doğrulaması
	var calculatedTotal float64
	for _, item := range req.CartItems {
		// Güncel fiyatı kontrol et
		var currentPrice float64
		priceQuery := "SELECT price FROM products WHERE id = $1 AND is_active = true"
		err = tx.QueryRow(priceQuery, item.ProductID).Scan(&currentPrice)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ürün fiyatı alınamadı"})
			return
		}

		// Fiyat değişikliği kontrolü (opsiyonel warning)
		totalPrice := currentPrice * float64(item.Quantity)
		calculatedTotal += totalPrice

		orderItemQuery := `
			INSERT INTO order_items (order_id, product_id, quantity, unit_price, total_price) 
			VALUES ($1, $2, $3, $4, $5)
		`
		_, err = tx.Exec(orderItemQuery, orderID, item.ProductID, item.Quantity,
			currentPrice, totalPrice)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Sipariş öğeleri eklenemedi: " + err.Error()})
			return
		}
	}

	// EKLEME: Total amount validation
	if calculatedTotal != req.TotalAmount {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "Fiyat uyuşmazlığı",
			"calculated": calculatedTotal,
			"requested":  req.TotalAmount,
		})
		return
	}

	// Sepeti temizle
	var cartID int
	err = tx.QueryRow("SELECT id FROM carts WHERE user_id = $1", userID).Scan(&cartID)
	if err == nil {
		_, err = tx.Exec("DELETE FROM cart_items WHERE cart_id = $1", cartID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Sepet temizlenemedi"})
			return
		}
	}

	// DÜZELTME: Sipariş durumu workflow - önce pending, sonra processing
	_, err = tx.Exec("UPDATE orders SET status = 'processing', updated_at = NOW() WHERE id = $1", orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sipariş durumu güncellenemedi"})
		return
	}

	// EKLEME: Stok rezervasyonu (stok düşürme yerine önce rezerve et)
	for _, item := range req.CartItems {
		_, err = tx.Exec(`
			UPDATE inventory 
			SET reserved_quantity = reserved_quantity + $1, updated_at = NOW() 
			WHERE product_id = $2
		`, item.Quantity, item.ProductID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Stok rezerve edilemedi"})
			return
		}
	}

	// Transaction commit
	if err = tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sipariş tamamlanamadı"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":      "Sipariş başarıyla oluşturuldu",
		"order_id":     orderID,
		"total_amount": calculatedTotal,
		"status":       "processing",
	})
}

// EKLEME: Supabase client ile alternatif cart items methodu (performans karşılaştırması için)
func (h *CartHandler) GetCartItemsWithSupabase(c *gin.Context) {
	userID := c.GetString("userID")

	// Supabase client ile query - doğru API kullanımı
	data, _, err := h.supabaseClient.From("cart_items").
		Select("*, products(*), carts!inner(user_id)", "", false).
		Eq("carts.user_id", userID).
		Eq("products.is_active", "true").
		Order("created_at", nil).
		Execute()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sepet öğeleri alınamadı (Supabase): " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": data,
		"note":  "Supabase client ile getirilen data",
	})
}

// ALTERNATIF: Daha basit Supabase query
func (h *CartHandler) GetCartItemsSimpleSupabase(c *gin.Context) {

	// Basit query - sadece cart_items
	data, count, err := h.supabaseClient.From("cart_items").
		Select("*", "exact", false).
		Execute()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Sepet öğeleri alınamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": data,
		"count": count,
		"note":  "Basit Supabase query",
	})
}
