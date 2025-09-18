// internal/handlers/comment.go - DÜZELTME
package handlers

import (
	"database/sql"
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type CommentHandler struct {
	cfg *config.Config
}

func NewCommentHandler(cfg *config.Config) *CommentHandler {
	return &CommentHandler{
		cfg: cfg,
	}
}

func (h *CommentHandler) GetComments(c *gin.Context) {
	productID, err := strconv.Atoi(c.Param("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz product ID"})
		return
	}

	userID := c.Query("user_id")

	// DÜZELTME: Prepared statement yerine direct query kullan
	var query string
	var args []interface{}

	if userID != "" {
		// User ID varsa like durumunu da kontrol et
		query = `
			SELECT 
				c.id, c.product_id, c.user_id, c.content, c.parent_comment_id, 
				c.created_at,
				COALESCE(p.full_name, p.username, SPLIT_PART(p.email, '@', 1), 'Anonim') as user_name,
				p.email as user_email,
				COUNT(cl.id) as likes_count,
				CASE WHEN EXISTS(
					SELECT 1 FROM comment_likes cl2 
					WHERE cl2.comment_id = c.id AND cl2.user_id = $2
				) THEN true ELSE false END as user_liked
			FROM comments c
			LEFT JOIN profiles p ON c.user_id = p.id
			LEFT JOIN comment_likes cl ON c.id = cl.comment_id
			WHERE c.product_id = $1
			GROUP BY c.id, c.product_id, c.user_id, c.content, c.parent_comment_id, 
					 c.created_at, p.full_name, p.username, p.email
			ORDER BY c.created_at DESC
		`
		args = []interface{}{productID, userID}
	} else {
		// User ID yoksa sadece temel bilgileri al
		query = `
			SELECT 
				c.id, c.product_id, c.user_id, c.content, c.parent_comment_id, 
				c.created_at,
				COALESCE(p.full_name, p.username, SPLIT_PART(p.email, '@', 1), 'Anonim') as user_name,
				p.email as user_email,
				COUNT(cl.id) as likes_count,
				false as user_liked
			FROM comments c
			LEFT JOIN profiles p ON c.user_id = p.id
			LEFT JOIN comment_likes cl ON c.id = cl.comment_id
			WHERE c.product_id = $1
			GROUP BY c.id, c.product_id, c.user_id, c.content, c.parent_comment_id, 
					 c.created_at, p.full_name, p.username, p.email
			ORDER BY c.created_at DESC
		`
		args = []interface{}{productID}
	}

	var rows *sql.Rows

	// DÜZELTME: Eğer args boşsa Query'yi parametresiz çağır
	if len(args) == 0 {
		rows, err = database.DB.Query(query)
	} else {
		rows, err = database.DB.Query(query, args...)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Yorumlar alınamadı: " + err.Error()})
		return
	}
	defer rows.Close()

	comments := make([]models.Comment, 0)
	for rows.Next() {
		var comment models.Comment
		err := rows.Scan(
			&comment.ID,
			&comment.ProductID,
			&comment.UserID,
			&comment.Content,
			&comment.ParentCommentID,
			&comment.CreatedAt,
			&comment.UserName,
			&comment.UserEmail,
			&comment.Likes,
			&comment.UserLiked,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Yorum verisi işlenemedi: " + err.Error()})
			return
		}
		comments = append(comments, comment)
	}

	c.JSON(http.StatusOK, gin.H{"comments": comments})
}

func (h *CommentHandler) AddComment(c *gin.Context) {
	userID := c.GetString("userID")

	var req models.AddCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Comment insert et
	query := `
        INSERT INTO comments (product_id, user_id, content, parent_comment_id) 
        VALUES ($1, $2, $3, $4) 
        RETURNING id, created_at
    `

	var comment models.Comment
	err := database.DB.QueryRow(query, req.ProductID, userID, req.Content, req.ParentID).Scan(
		&comment.ID, &comment.CreatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Yorum eklenemedi"})
		return
	}

	// Kullanıcı bilgilerini al
	userQuery := `
        SELECT COALESCE(full_name, username, SPLIT_PART(email, '@', 1), 'Anonim'), email 
        FROM profiles WHERE id = $1
    `
	database.DB.QueryRow(userQuery, userID).Scan(&comment.UserName, &comment.UserEmail)

	// Response'u tamamla
	comment.ProductID = req.ProductID
	comment.UserID = userID
	comment.Content = req.Content
	comment.ParentCommentID = req.ParentID
	comment.Likes = 0

	c.JSON(http.StatusCreated, comment)
}

func (h *CommentHandler) ToggleLike(c *gin.Context) {
	userID := c.GetString("userID")
	commentID, err := strconv.Atoi(c.Param("commentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz comment ID"})
		return
	}

	// Mevcut beğeniyi kontrol et
	var existingID int
	checkQuery := "SELECT id FROM comment_likes WHERE comment_id = $1 AND user_id = $2"
	err = database.DB.QueryRow(checkQuery, commentID, userID).Scan(&existingID)

	if err == nil {
		// Beğeniyi kaldır
		deleteQuery := "DELETE FROM comment_likes WHERE id = $1"
		_, err = database.DB.Exec(deleteQuery, existingID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Beğeni kaldırılamadı"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Beğeni kaldırıldı"})
	} else {
		// Beğeni ekle
		insertQuery := "INSERT INTO comment_likes (comment_id, user_id) VALUES ($1, $2)"
		_, err = database.DB.Exec(insertQuery, commentID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Beğeni eklenemedi"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Beğeni eklendi"})
	}
}
