// internal/models/models.go
package models

import (
	"time"
)

type User struct {
	ID        string    `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Profile struct {
	ID        string    `json:"id" db:"id"`
	FullName  *string   `json:"full_name" db:"full_name"`
	Username  *string   `json:"username" db:"username"`
	Email     string    `json:"email" db:"email"`
	IsAdmin   bool      `json:"is_admin" db:"is_admin"`
	AvatarURL *string   `json:"avatar_url" db:"avatar_url"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type Product struct {
	ID          int       `json:"id" db:"id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	Price       float64   `json:"price" db:"price"`
	Image       string    `json:"image" db:"image"`
	Category    string    `json:"category" db:"category"`
	SKU         string    `json:"sku" db:"sku"`
	Rating      float64   `json:"rating" db:"rating"`
	RatingCount int       `json:"rating_count" db:"rating_count"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Inventory struct {
	ID               int       `json:"id" db:"id"`
	ProductID        int       `json:"product_id" db:"product_id"`
	Quantity         int       `json:"quantity" db:"quantity"`
	ReservedQuantity int       `json:"reserved_quantity" db:"reserved_quantity"`
	MinStockLevel    int       `json:"min_stock_level" db:"min_stock_level"`
	MaxStockLevel    int       `json:"max_stock_level" db:"max_stock_level"`
	CostPrice        float64   `json:"cost_price" db:"cost_price"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type ProductWithStock struct {
	Product
	Inventory      *Inventory `json:"inventory"`
	AvailableStock int        `json:"available_stock"`
	StockStatus    string     `json:"stock_status"`
}

type Cart struct {
	ID        int       `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type CartItem struct {
	ID        int       `json:"id" db:"id"`
	CartID    int       `json:"cart_id" db:"cart_id"`
	ProductID int       `json:"product_id" db:"product_id"`
	Quantity  int       `json:"quantity" db:"quantity"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Product   *Product  `json:"product,omitempty"`
}

type Comment struct {
	ID              int       `json:"id" db:"id"`
	ProductID       int       `json:"product_id" db:"product_id"`
	UserID          string    `json:"user_id" db:"user_id"`
	Content         string    `json:"content" db:"content"`
	ParentCommentID *int      `json:"parent_comment_id" db:"parent_comment_id"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	UserName        string    `json:"user_name,omitempty"`
	UserEmail       string    `json:"user_email,omitempty"`
	Likes           int       `json:"likes,omitempty"`
	UserLiked       bool      `json:"user_liked,omitempty"`
}

type CommentLike struct {
	ID        int       `json:"id" db:"id"`
	CommentID int       `json:"comment_id" db:"comment_id"`
	UserID    string    `json:"user_id" db:"user_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Order struct {
	ID          int       `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	TotalAmount float64   `json:"total_amount" db:"total_amount"`
	Status      string    `json:"status" db:"status"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type OrderItem struct {
	ID         int     `json:"id" db:"id"`
	OrderID    int     `json:"order_id" db:"order_id"`
	ProductID  int     `json:"product_id" db:"product_id"`
	Quantity   int     `json:"quantity" db:"quantity"`
	UnitPrice  float64 `json:"unit_price" db:"unit_price"`
	TotalPrice float64 `json:"total_price" db:"total_price"`
}

// Request/Response DTOs
type SignInRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type SignUpRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type AddCommentRequest struct {
	ProductID int    `json:"product_id" binding:"required"`
	Content   string `json:"content" binding:"required"`
	ParentID  *int   `json:"parent_id"`
}

type AddCartItemRequest struct {
	ProductID int `json:"product_id" binding:"required"`
	Quantity  int `json:"quantity" binding:"required,min=1"`
}

type CheckStockRequest struct {
	Quantity int `json:"quantity" binding:"required,min=1"`
}

type CreateOrderRequest struct {
	CartItems   []CartItem `json:"cart_items" binding:"required"`
	TotalAmount float64    `json:"total_amount" binding:"required"`
}
