package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/google"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type AuthServiceInterface interface {
	Register(ctx context.Context, email, password, name string) (*model.AuthResponse, error)
	Login(ctx context.Context, email, password string) (*model.AuthResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*model.AuthResponse, error)
	AuthenticateGoogleUser(ctx context.Context, email, name, avatarURL string) (*model.AuthResponse, error)
}

type GoogleClientInterface interface {
	ExchangeCode(ctx context.Context, code, redirectURI string) (*google.UserInfo, error)
}

type TokenDenylistInterface interface {
	Revoke(ctx context.Context, token string, ttl time.Duration) error
}

// CookieConfig controls cookie attributes per environment.
type CookieConfig struct {
	Secure   bool
	Domain   string
	SameSite http.SameSite
}

type AuthHandler struct {
	svc          AuthServiceInterface
	googleClient GoogleClientInterface
	denylist     TokenDenylistInterface
	accessTTL    time.Duration
	refreshTTL   time.Duration
	cookieCfg    CookieConfig
}

func NewAuthHandler(svc AuthServiceInterface, googleClient GoogleClientInterface, denylist TokenDenylistInterface, accessTTL, refreshTTL time.Duration, cookieCfg CookieConfig) *AuthHandler {
	return &AuthHandler{svc: svc, googleClient: googleClient, denylist: denylist, accessTTL: accessTTL, refreshTTL: refreshTTL, cookieCfg: cookieCfg}
}

func (h *AuthHandler) setAuthCookies(c *gin.Context, resp *model.AuthResponse) {
	c.SetSameSite(h.cookieCfg.SameSite)
	c.SetCookie("access_token", resp.AccessToken, int(h.accessTTL.Seconds()), "/", h.cookieCfg.Domain, h.cookieCfg.Secure, true)
	c.SetCookie("refresh_token", resp.RefreshToken, int(h.refreshTTL.Seconds()), "/go-auth/auth", h.cookieCfg.Domain, h.cookieCfg.Secure, true)
}

func (h *AuthHandler) clearAuthCookies(c *gin.Context) {
	c.SetSameSite(h.cookieCfg.SameSite)
	c.SetCookie("access_token", "", -1, "/", h.cookieCfg.Domain, h.cookieCfg.Secure, true)
	c.SetCookie("refresh_token", "", -1, "/go-auth/auth", h.cookieCfg.Domain, h.cookieCfg.Secure, true)
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	resp, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		_ = c.Error(err)
		return
	}
	h.setAuthCookies(c, resp)
	c.JSON(http.StatusOK, gin.H{
		"userId":    resp.UserID,
		"email":     resp.Email,
		"name":      resp.Name,
		"avatarUrl": resp.AvatarURL,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		_ = c.Error(err)
		return
	}
	h.setAuthCookies(c, resp)
	c.JSON(http.StatusOK, gin.H{
		"userId":    resp.UserID,
		"email":     resp.Email,
		"name":      resp.Name,
		"avatarUrl": resp.AvatarURL,
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		// Fallback to JSON body for backward compatibility
		var req model.RefreshRequest
		if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
			_ = c.Error(apperror.Unauthorized("MISSING_TOKEN", "missing refresh token"))
			return
		}
		refreshToken = req.RefreshToken
	}
	resp, errSvc := h.svc.Refresh(c.Request.Context(), refreshToken)
	if errSvc != nil {
		_ = c.Error(errSvc)
		return
	}
	h.setAuthCookies(c, resp)
	c.JSON(http.StatusOK, gin.H{
		"userId":    resp.UserID,
		"email":     resp.Email,
		"name":      resp.Name,
		"avatarUrl": resp.AvatarURL,
	})
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	var req model.GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	info, err := h.googleClient.ExchangeCode(c.Request.Context(), req.Code, req.RedirectURI)
	if err != nil {
		_ = c.Error(apperror.Unauthorized("GOOGLE_AUTH_FAILED", "google authentication failed"))
		return
	}
	resp, err := h.svc.AuthenticateGoogleUser(c.Request.Context(), info.Email, info.Name, info.Picture)
	if err != nil {
		_ = c.Error(err)
		return
	}
	h.setAuthCookies(c, resp)
	c.JSON(http.StatusOK, gin.H{
		"userId":    resp.UserID,
		"email":     resp.Email,
		"name":      resp.Name,
		"avatarUrl": resp.AvatarURL,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token, _ := c.Cookie("access_token")
	if token == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if token != "" && h.denylist != nil {
		_ = h.denylist.Revoke(c.Request.Context(), token, h.accessTTL)
	}
	h.clearAuthCookies(c)
	c.Status(http.StatusNoContent)
}
