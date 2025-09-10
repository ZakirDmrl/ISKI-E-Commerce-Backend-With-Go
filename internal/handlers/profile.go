package handlers

import (
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ProfileHandler struct {
	cfg *config.Config
}

func NewProfileHandler(cfg *config.Config) *ProfileHandler {
	return &ProfileHandler{cfg: cfg}
}

func (h *ProfileHandler) GetProfile(c *gin.Context) {
	userID := c.GetString("userID")

	query := `
		SELECT id, full_name, username, email, is_admin, avatar_url, created_at, updated_at 
		FROM profiles WHERE id = $1
	`

	var profile models.Profile
	err := database.DB.QueryRow(query, userID).Scan(
		&profile.ID, &profile.FullName, &profile.Username, &profile.Email,
		&profile.IsAdmin, &profile.AvatarURL, &profile.CreatedAt, &profile.UpdatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profil bulunamadı"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"profile": profile})
}
func (h *ProfileHandler) UploadAvatar(c *gin.Context) {
	userID := c.GetString("userID")

	// Multipart form'dan file al
	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar dosyası gerekli"})
		return
	}
	defer file.Close()

	// File size kontrolü (5MB)
	if header.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dosya boyutu çok büyük (max 5MB)"})
		return
	}

	// File type kontrolü
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sadece resim dosyaları kabul edilir"})
		return
	}

	// File extension al
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}

	// Unique filename oluştur
	filename := fmt.Sprintf("avatar_%s_%d%s", userID, time.Now().Unix(), ext)

	// Upload directory oluştur
	uploadDir := "uploads/avatars"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload directory oluşturulamadı"})
		return
	}

	// Eski avatar dosyasını sil
	var oldAvatarURL string
	database.DB.QueryRow("SELECT avatar_url FROM profiles WHERE id = $1", userID).Scan(&oldAvatarURL)
	if oldAvatarURL != "" && strings.Contains(oldAvatarURL, "/uploads/") {
		oldPath := "." + strings.Split(oldAvatarURL, "/uploads/")[1]
		os.Remove("uploads/" + oldPath)
	}

	// Dosyayı kaydet
	filePath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Dosya kaydedilemedi"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Dosya kopyalanamadı"})
		return
	}

	// Public URL oluştur
	avatarURL := fmt.Sprintf("/uploads/avatars/%s", filename)

	// Database'de avatar URL'ini güncelle
	updateQuery := `
        UPDATE profiles 
        SET avatar_url = $1, updated_at = NOW() 
        WHERE id = $2 
        RETURNING avatar_url
    `

	var updatedURL string
	err = database.DB.QueryRow(updateQuery, avatarURL, userID).Scan(&updatedURL)
	if err != nil {
		// Yüklenen dosyayı sil
		os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Avatar URL güncellenemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Avatar başarıyla yüklendi",
		"avatar_url": updatedURL,
	})
}
func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetString("userID")

	var req struct {
		FullName  *string `json:"full_name"`
		Username  *string `json:"username"`
		AvatarURL *string `json:"avatar_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Username benzersizlik kontrolü
	if req.Username != nil && *req.Username != "" {
		var existingID string
		checkQuery := "SELECT id FROM profiles WHERE username = $1 AND id != $2"
		err := database.DB.QueryRow(checkQuery, *req.Username, userID).Scan(&existingID)
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Bu kullanıcı adı zaten kullanımda"})
			return
		}
	}

	// Profile güncelle
	updateQuery := `
		UPDATE profiles 
		SET full_name = $1, username = $2, avatar_url = $3, updated_at = NOW() 
		WHERE id = $4 
		RETURNING id, full_name, username, email, is_admin, avatar_url, created_at, updated_at
	`

	var profile models.Profile
	err := database.DB.QueryRow(updateQuery, req.FullName, req.Username, req.AvatarURL, userID).Scan(
		&profile.ID, &profile.FullName, &profile.Username, &profile.Email,
		&profile.IsAdmin, &profile.AvatarURL, &profile.CreatedAt, &profile.UpdatedAt,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Profil güncellenemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profil başarıyla güncellendi",
		"profile": profile,
	})
}
