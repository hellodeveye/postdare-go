package util

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const RequestIDKey = "request_id"

type Pagination struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req_" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return "req_" + hex.EncodeToString(b[:])
}

func RequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"data": data, "request_id": RequestID(c)})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"data": data, "request_id": RequestID(c)})
}

func Accepted(c *gin.Context, data interface{}) {
	c.JSON(http.StatusAccepted, gin.H{"data": data, "request_id": RequestID(c)})
}

func List(c *gin.Context, data interface{}, p Pagination) {
	c.JSON(http.StatusOK, gin.H{"data": data, "pagination": p, "request_id": RequestID(c)})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Error(c *gin.Context, status int, code, message string, details interface{}) {
	c.JSON(status, gin.H{
		"error":      ErrorBody{Code: code, Message: message, Details: details},
		"request_id": RequestID(c),
	})
}

func ParsePagination(c *gin.Context) (page int, pageSize int, offset int) {
	page = parsePositive(c.Query("page"), 1)
	pageSize = parsePositive(c.Query("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

func ParseLines(raw string, def int, max int) int {
	lines := parsePositive(raw, def)
	if lines > max {
		return max
	}
	return lines
}

func parsePositive(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func SortClause(raw string, allowed map[string]bool, fallback string) string {
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 2 || !allowed[parts[0]] {
		return fallback
	}
	dir := strings.ToLower(parts[1])
	if dir != "asc" && dir != "desc" {
		return fallback
	}
	return parts[0] + " " + dir
}

func MaskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "******"
	}
	return s[:3] + "******" + s[len(s)-3:]
}
