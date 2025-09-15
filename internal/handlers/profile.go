package handlers

import (
	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"
	"fmt"
	"io"
	"net/http"
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

	if h.cfg.SupabaseURL == "" || (h.cfg.SupabaseKey == "" && h.cfg.SupabaseServiceKey == "") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Supabase yapılandırması eksik"})
		return
	}

	// Kullanılacak anahtar (service role varsa onu kullan)
	authKey := h.cfg.SupabaseServiceKey
	if authKey == "" {
		authKey = h.cfg.SupabaseKey
	}

	// Eski avatar'ı Supabase'den sil (kullanıcı klasörü içindeyse)
	var oldAvatarURL string
	database.DB.QueryRow("SELECT avatar_url FROM profiles WHERE id = $1", userID).Scan(&oldAvatarURL)
	if oldAvatarURL != "" {
		prefix := "/storage/v1/object/public/avatars/"
		if idx := strings.Index(oldAvatarURL, prefix); idx != -1 {
			rel := oldAvatarURL[idx+len(prefix):]
			if strings.HasPrefix(rel, userID+"/") {
				deleteURL := strings.TrimRight(h.cfg.SupabaseURL, "/") + "/storage/v1/object/avatars/" + rel
				req, _ := http.NewRequest(http.MethodDelete, deleteURL, nil)
				req.Header.Set("Authorization", "Bearer "+authKey)
				req.Header.Set("apikey", authKey)
				_, _ = http.DefaultClient.Do(req)
			}
		}
	}

	// avatars/{userID}/{filename} içine yükle
	objectPath := filepath.ToSlash(filepath.Join(userID, filename))
	uploadURL := strings.TrimRight(h.cfg.SupabaseURL, "/") + "/storage/v1/object/avatars/" + objectPath + "?upsert=true"

	req, err := http.NewRequest(http.MethodPost, uploadURL, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload isteği oluşturulamadı"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+authKey)
	req.Header.Set("apikey", authKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Supabase'a bağlanılamadı"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Supabase yükleme hatası: %s", strings.TrimSpace(string(body)))})
		return
	}

	// Public URL oluştur (bucket public ise)
	publicURL := strings.TrimRight(h.cfg.SupabaseURL, "/") + "/storage/v1/object/public/avatars/" + objectPath

	// Database'de avatar URL'ini güncelle
	updateQuery := `
        UPDATE profiles 
        SET avatar_url = $1, updated_at = NOW() 
        WHERE id = $2 
        RETURNING avatar_url
    `

	var updatedURL string
	err = database.DB.QueryRow(updateQuery, publicURL, userID).Scan(&updatedURL)
	if err != nil {
		// Geri al
		deleteURL := strings.TrimRight(h.cfg.SupabaseURL, "/") + "/storage/v1/object/avatars/" + objectPath
		req2, _ := http.NewRequest(http.MethodDelete, deleteURL, nil)
		req2.Header.Set("Authorization", "Bearer "+authKey)
		req2.Header.Set("apikey", authKey)
		_, _ = http.DefaultClient.Do(req2)

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
