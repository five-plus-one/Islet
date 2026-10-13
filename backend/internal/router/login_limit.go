package router

import (
	"github.com/gin-gonic/gin"
	"islet/backend/internal/response"
	"net/http"
	"sync"
	"time"
)

func localLoginLimit() gin.HandlerFunc {
	type entry struct {
		count int
		until time.Time
	}
	var mu sync.Mutex
	entries := map[string]entry{}
	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP()
		mu.Lock()
		for k, v := range entries {
			if now.After(v.until) {
				delete(entries, k)
			}
		}
		v := entries[key]
		if v.until.IsZero() {
			v.until = now.Add(time.Minute)
		}
		v.count++
		blocked := v.count > 30 || len(entries) > 10000
		entries[key] = v
		mu.Unlock()
		if blocked {
			c.Header("Retry-After", "60")
			response.Error(c, http.StatusTooManyRequests, 429, "登录尝试过于频繁，请一分钟后重试")
			c.Abort()
			return
		}
		c.Next()
	}
}
