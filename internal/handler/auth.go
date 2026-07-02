package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/hellodeveye/postdare-go/internal/middleware"
	"github.com/hellodeveye/postdare-go/internal/model"
	"github.com/hellodeveye/postdare-go/internal/util"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid login request", err.Error())
		return
	}
	var user model.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		util.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password", nil)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		util.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password", nil)
		return
	}
	expires := time.Now().Add(h.Config.JWTDuration())
	claims := middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   strconv.FormatUint(user.ID, 10),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.Config.JWT.Secret))
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "TOKEN_CREATE_FAILED", "Failed to create token", nil)
		return
	}
	util.OK(c, gin.H{"token": token, "expires_at": expires, "user": user})
}

func (h *Handler) Me(c *gin.Context) {
	if middleware.IsMCP(c) {
		util.OK(c, gin.H{
			"role":                 c.GetString(middleware.RoleKey),
			"actor":                c.GetString(middleware.ActorKey),
			"must_change_password": false,
		})
		return
	}
	userID, _ := c.Get(middleware.UserIDKey)
	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not found", nil)
		return
	}
	util.OK(c, gin.H{
		"id":                   user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"actor":                c.GetString(middleware.ActorKey),
		"must_change_password": user.MustChangePassword,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	util.OK(c, gin.H{"ok": true})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID, ok := c.Get(middleware.UserIDKey)
	if !ok {
		util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "User token is required", nil)
		return
	}
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid password request", err.Error())
		return
	}
	if len(req.NewPassword) < 8 {
		util.Error(c, http.StatusBadRequest, "INVALID_PASSWORD", "New password must be at least 8 characters", nil)
		return
	}
	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not found", nil)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		util.Error(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Old password is incorrect", nil)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "PASSWORD_UPDATE_FAILED", "Failed to update password", nil)
		return
	}
	if err := h.DB.Model(&user).Updates(map[string]interface{}{
		"password_hash":        string(hash),
		"must_change_password": false,
	}).Error; err != nil {
		util.Error(c, http.StatusInternalServerError, "PASSWORD_UPDATE_FAILED", "Failed to update password", nil)
		return
	}
	util.OK(c, gin.H{"ok": true})
}

func (h *Handler) RequirePasswordReady(c *gin.Context) {
	if middleware.IsMCP(c) {
		c.Next()
		return
	}
	userID, ok := c.Get(middleware.UserIDKey)
	if !ok {
		util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "User token is required", nil)
		c.Abort()
		return
	}
	var user model.User
	if err := h.DB.Select("id", "must_change_password").First(&user, userID).Error; err != nil {
		util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not found", nil)
		c.Abort()
		return
	}
	if user.MustChangePassword {
		util.Error(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "Password change is required before continuing", nil)
		c.Abort()
		return
	}
	c.Next()
}
