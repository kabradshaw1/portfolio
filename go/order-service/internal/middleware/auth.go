package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else if cookie, err := c.Cookie("access_token"); err == nil && cookie != "" {
			tokenStr = cookie
		}
		if tokenStr == "" {
			_ = c.Error(apperror.Unauthorized("MISSING_AUTH", "missing authorization"))
			c.Abort()
			return
		}
		claims := jwt.MapClaims{}
		_, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, apperror.Forbidden("INVALID_TOKEN", "unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})
		if err != nil {
			_ = c.Error(apperror.Forbidden("INVALID_TOKEN", "invalid or expired token"))
			c.Abort()
			return
		}
		sub, ok := claims["sub"].(string)
		if !ok {
			_ = c.Error(apperror.Forbidden("INVALID_TOKEN", "invalid token claims"))
			c.Abort()
			return
		}
		c.Set("userId", sub)
		c.Next()
	}
}
