// ========================================
// internal/handlers/auth.go - DOĞRU SUPABASE CLIENT KULLANIMI
// ========================================
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ecommerce-backend/internal/config"
	"ecommerce-backend/internal/database"
	"ecommerce-backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/supabase-community/supabase-go"
)

type AuthHandler struct {
	supabaseClient *supabase.Client // Database operasyonları için
	supabaseURL    string           // Auth API çağrıları için
	supabaseKey    string           // Auth API çağrıları için
	jwtSecret      string
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	client, err := supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseKey, nil)
	if err != nil {
		panic(fmt.Sprintf("Supabase client oluşturulamadı: %v", err))
	}

	return &AuthHandler{
		supabaseClient: client,
		supabaseURL:    cfg.SupabaseURL,
		supabaseKey:    cfg.SupabaseKey,
		jwtSecret:      cfg.JWTSecret,
	}
}

// Supabase Auth API için HTTP helper
func (h *AuthHandler) makeAuthRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, h.supabaseURL+"/auth/v1"+endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", h.supabaseKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("auth request failed: %s", string(responseBody))
	}

	return responseBody, nil
}

// Authorized request helper (token ile)
func (h *AuthHandler) makeAuthRequestWithToken(method, endpoint, token string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, h.supabaseURL+"/auth/v1"+endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("apikey", h.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("auth request failed: %s", string(responseBody))
	}

	return responseBody, nil
}

func (h *AuthHandler) SignIn(c *gin.Context) {
	var req models.SignInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Supabase Auth API ile giriş
	credentials := map[string]interface{}{
		"email":    req.Email,
		"password": req.Password,
	}

	responseBody, err := h.makeAuthRequest("POST", "/token?grant_type=password", credentials)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Giriş başarısız: " + err.Error()})
		return
	}

	var authResponse struct {
		User struct {
			ID               string `json:"id"`
			Email            string `json:"email"`
			CreatedAt        string `json:"created_at"`
			UpdatedAt        string `json:"updated_at"`
			Aud              string `json:"aud"`
			Role             string `json:"role"`
			EmailConfirmedAt string `json:"email_confirmed_at,omitempty"`
			LastSignInAt     string `json:"last_sign_in_at,omitempty"`
		} `json:"user"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Session      *struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			TokenType    string `json:"token_type"`
		} `json:"session"`
	}

	if err := json.Unmarshal(responseBody, &authResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response parse edilemedi"})
		return
	}

	// Profil bilgilerini SQL ile çek (Supabase client yerine direkt SQL)
	var profile models.Profile
	profileQuery := `
		SELECT id, full_name, username, email, is_admin, avatar_url, created_at, updated_at 
		FROM profiles WHERE id = $1
	`
	err = database.DB.QueryRow(profileQuery, authResponse.User.ID).Scan(
		&profile.ID, &profile.FullName, &profile.Username, &profile.Email,
		&profile.IsAdmin, &profile.AvatarURL, &profile.CreatedAt, &profile.UpdatedAt,
	)

	if err != nil {
		// Profil yoksa oluştur
		insertQuery := `
			INSERT INTO profiles (id, email, is_admin, created_at, updated_at) 
			VALUES ($1, $2, false, NOW(), NOW()) 
			RETURNING id, full_name, username, email, is_admin, avatar_url, created_at, updated_at
		`
		err = database.DB.QueryRow(insertQuery, authResponse.User.ID, authResponse.User.Email).Scan(
			&profile.ID, &profile.FullName, &profile.Username, &profile.Email,
			&profile.IsAdmin, &profile.AvatarURL, &profile.CreatedAt, &profile.UpdatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Profil oluşturulamadı: " + err.Error()})
			return
		}
	}

	// AppUser formatında response oluştur
	appUser := gin.H{
		"id":         authResponse.User.ID,
		"email":      authResponse.User.Email,
		"created_at": authResponse.User.CreatedAt,
		"updated_at": authResponse.User.UpdatedAt,
		"isAdmin":    profile.IsAdmin,
		"avatarUrl":  profile.AvatarURL,
		"aud":        authResponse.User.Aud,
		"role":       authResponse.User.Role,
	}

	// Token'lar hem root hem de session içinde gelebilir; güvenle taşı
	accessToken := authResponse.AccessToken
	refreshToken := authResponse.RefreshToken
	expiresIn := authResponse.ExpiresIn
	tokenType := authResponse.TokenType
	if accessToken == "" && authResponse.Session != nil {
		accessToken = authResponse.Session.AccessToken
		refreshToken = authResponse.Session.RefreshToken
		expiresIn = authResponse.Session.ExpiresIn
		tokenType = authResponse.Session.TokenType
	}

	response := gin.H{
		"user": appUser,
		"session": gin.H{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"expires_in":    expiresIn,
			"token_type":    tokenType,
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) SignUp(c *gin.Context) {
	var req models.SignUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Supabase Auth API ile kayıt
	credentials := map[string]interface{}{
		"email":    req.Email,
		"password": req.Password,
	}

	responseBody, err := h.makeAuthRequest("POST", "/signup", credentials)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Kayıt başarısız: " + err.Error()})
		return
	}

	var signupResponse struct {
		User struct {
			ID        string `json:"id"`
			Email     string `json:"email"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Aud       string `json:"aud"`
			Role      string `json:"role"`
		} `json:"user"`
		Session *struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			TokenType    string `json:"token_type"`
		} `json:"session"`
	}

	if err := json.Unmarshal(responseBody, &signupResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response parse edilemedi"})
		return
	}

	// AppUser formatında response oluştur
	var appUser gin.H
	if signupResponse.User.ID != "" {
		appUser = gin.H{
			"id":         signupResponse.User.ID,
			"email":      signupResponse.User.Email,
			"created_at": signupResponse.User.CreatedAt,
			"updated_at": signupResponse.User.UpdatedAt,
			"isAdmin":    false,
			"avatarUrl":  nil,
			"aud":        signupResponse.User.Aud,
			"role":       signupResponse.User.Role,
		}
	}

	response := gin.H{
		"user":    appUser,
		"message": "Kayıt başarılı. Email adresinizi doğrulayın.",
	}

	c.JSON(http.StatusCreated, response)
}

func (h *AuthHandler) SignOut(c *gin.Context) {
	// Token'ı al
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Supabase logout endpoint'ini çağır
		_, err := h.makeAuthRequestWithToken("POST", "/logout", token, nil)
		if err != nil {
			// Logout hatası olsa bile client-side'da token silinecek
			c.JSON(http.StatusOK, gin.H{"message": "Çıkış yapıldı"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Başarıyla çıkış yapıldı"})
}

func (h *AuthHandler) GetMe(c *gin.Context) {
	userID := c.GetString("userID")

	// Token'ı al
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header gerekli"})
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Supabase'den user bilgisini al
	responseBody, err := h.makeAuthRequestWithToken("GET", "/user", token, nil)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Geçersiz token: " + err.Error()})
		return
	}

	var userResponse struct {
		ID               string `json:"id"`
		Email            string `json:"email"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
		Aud              string `json:"aud"`
		Role             string `json:"role"`
		EmailConfirmedAt string `json:"email_confirmed_at,omitempty"`
		LastSignInAt     string `json:"last_sign_in_at,omitempty"`
	}

	if err := json.Unmarshal(responseBody, &userResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User response parse edilemedi"})
		return
	}

	// Profil bilgilerini SQL ile çek
	var profile models.Profile
	profileQuery := `
		SELECT id, full_name, username, email, is_admin, avatar_url, created_at, updated_at 
		FROM profiles WHERE id = $1
	`
	err = database.DB.QueryRow(profileQuery, userID).Scan(
		&profile.ID, &profile.FullName, &profile.Username, &profile.Email,
		&profile.IsAdmin, &profile.AvatarURL, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Kullanıcı profili bulunamadı: " + err.Error()})
		return
	}

	// AppUser formatında response oluştur
	appUser := gin.H{
		"id":         userResponse.ID,
		"email":      userResponse.Email,
		"created_at": userResponse.CreatedAt,
		"updated_at": userResponse.UpdatedAt,
		"isAdmin":    profile.IsAdmin,
		"avatarUrl":  profile.AvatarURL,
		"aud":        userResponse.Aud,
		"role":       userResponse.Role,
	}

	c.JSON(http.StatusOK, gin.H{"user": appUser})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Supabase token refresh
	refreshData := map[string]interface{}{
		"refresh_token": req.RefreshToken,
	}

	responseBody, err := h.makeAuthRequest("POST", "/token?grant_type=refresh_token", refreshData)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token yenilenemedi: " + err.Error()})
		return
	}

	var refreshResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}

	if err := json.Unmarshal(responseBody, &refreshResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token response parse edilemedi"})
		return
	}

	c.JSON(http.StatusOK, refreshResponse)
}

// ========================================
// Supabase Client'ı Database İçin Kullanma Örneği
// ========================================

// Eğer database operasyonları için Supabase client kullanmak isterseniz:
func (h *AuthHandler) GetUserProfilesWithSupabase() ([]models.Profile, error) {
	// Supabase Go client sadece database operasyonları için kullanılır
	data, _, err := h.supabaseClient.From("profiles").Select("*", "", false).Execute()
	if err != nil {
		return nil, err
	}

	var profiles []models.Profile
	err = json.Unmarshal(data, &profiles)
	return profiles, err
}
