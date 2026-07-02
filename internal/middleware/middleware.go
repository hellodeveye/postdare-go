package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/hellodeveye/postdare-go/internal/config"
	"github.com/hellodeveye/postdare-go/internal/util"
)

const (
	UserIDKey   = "user_id"
	UsernameKey = "username"
	RoleKey     = "role"
	ActorKey    = "actor"
	ActorUser   = "user"
	ActorMCP    = "mcp"
)

type Claims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = util.NewRequestID()
		}
		c.Set(util.RequestIDKey, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("http request",
			zap.String("request_id", util.RequestID(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func CORS(origins []string) gin.HandlerFunc {
	if len(origins) == 0 {
		origins = []string{"http://localhost:5173"}
	}
	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-Request-ID", "X-GitHub-Event", "X-GitHub-Delivery", "X-Hub-Signature-256", "X-Gitee-Token", "X-Git-Osc-Token"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

func Auth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		tokenValue := ""
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			tokenValue = strings.TrimSpace(auth[len("Bearer "):])
		}
		if tokenValue == "" && strings.HasSuffix(c.Request.URL.Path, "/stream") {
			tokenValue = strings.TrimSpace(c.Query("access_token"))
		}
		if tokenValue == "" {
			util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing bearer token", nil)
			c.Abort()
			return
		}
		if cfg.MCP.Enabled && cfg.MCP.APIToken != "" && subtle.ConstantTimeCompare([]byte(tokenValue), []byte(cfg.MCP.APIToken)) == 1 {
			c.Set(ActorKey, ActorMCP)
			c.Set(RoleKey, "mcp")
			c.Next()
			return
		}
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenValue, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWT.Secret), nil
		})
		if err != nil || !token.Valid {
			util.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid bearer token", nil)
			c.Abort()
			return
		}
		c.Set(ActorKey, ActorUser)
		c.Set(UserIDKey, claims.UserID)
		c.Set(UsernameKey, claims.Username)
		c.Set(RoleKey, claims.Role)
		c.Next()
	}
}

func IsMCP(c *gin.Context) bool {
	v, _ := c.Get(ActorKey)
	return v == ActorMCP
}
