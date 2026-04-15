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

type AuthHandler struct {
	svc          AuthServiceInterface
	googleClient GoogleClientInterface
	denylist     TokenDenylistInterface
	accessTTL    time.Duration
}

func NewAuthHandler(svc AuthServiceInterface, googleClient GoogleClientInterface, denylist TokenDenylistInterface, accessTTL time.Duration) *AuthHandler {
	return &AuthHandler{svc: svc, googleClient: googleClient, denylist: denylist, accessTTL: accessTTL}
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
	c.JSON(http.StatusOK, resp)
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
	c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, resp)
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
	c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		_ = h.denylist.Revoke(c.Request.Context(), token, h.accessTTL)
	}
	c.Status(http.StatusNoContent)
}
