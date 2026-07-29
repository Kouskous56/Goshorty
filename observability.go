package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

type metricKey struct {
	Method string
	Route  string
	Status int
}

type requestMetric struct {
	Count         uint64
	DurationNanos uint64
}

// HTTPMetrics stores bounded-cardinality request aggregates. Route labels come
// from Gin's registered route templates, never raw URLs or short codes.
type HTTPMetrics struct {
	mu       sync.RWMutex
	requests map[metricKey]requestMetric
}

func NewHTTPMetrics() *HTTPMetrics {
	return &HTTPMetrics{requests: make(map[metricKey]requestMetric)}
}

func (m *HTTPMetrics) Observe(method, route string, status int, duration time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	key := metricKey{Method: method, Route: route, Status: status}
	m.mu.Lock()
	value := m.requests[key]
	value.Count++
	value.DurationNanos += uint64(duration)
	m.requests[key] = value
	m.mu.Unlock()
}

func (m *HTTPMetrics) Handler(c *gin.Context) {
	m.mu.RLock()
	keys := make([]metricKey, 0, len(m.requests))
	snapshot := make(map[metricKey]requestMetric, len(m.requests))
	for key := range m.requests {
		keys = append(keys, key)
		snapshot[key] = m.requests[key]
	}
	m.mu.RUnlock()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Route != keys[j].Route {
			return keys[i].Route < keys[j].Route
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Status < keys[j].Status
	})
	var output strings.Builder
	output.WriteString("# HELP goshorty_http_requests_total Total HTTP requests.\n")
	output.WriteString("# TYPE goshorty_http_requests_total counter\n")
	output.WriteString("# HELP goshorty_http_request_duration_seconds Request duration summary.\n")
	output.WriteString("# TYPE goshorty_http_request_duration_seconds summary\n")
	for _, key := range keys {
		value := snapshot[key]
		labels := fmt.Sprintf(`method=%q,route=%q,status=%q`,
			key.Method, key.Route, strconv.Itoa(key.Status))
		fmt.Fprintf(&output, "goshorty_http_requests_total{%s} %d\n", labels, value.Count)
		fmt.Fprintf(&output, "goshorty_http_request_duration_seconds_sum{%s} %.9f\n",
			labels, float64(value.DurationNanos)/float64(time.Second))
		fmt.Fprintf(&output, "goshorty_http_request_duration_seconds_count{%s} %d\n",
			labels, value.Count)
	}
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(output.String()))
}

func observabilityMiddleware(logger *slog.Logger, metrics *HTTPMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := c.GetHeader(requestIDHeader)
		if !validRequestID.MatchString(requestID) {
			requestID = newRequestID()
		}
		c.Set("request_id", requestID)
		c.Header(requestIDHeader, requestID)

		c.Next()

		duration := time.Since(started)
		route := c.FullPath()
		metrics.Observe(c.Request.Method, route, c.Writer.Status(), duration)
		logger.Info("http_request",
			"request_id", requestID,
			"method", c.Request.Method,
			"route", routeLabel(route),
			"status", c.Writer.Status(),
			"duration_ms", float64(duration.Microseconds())/1000,
			"client_ip", c.ClientIP(),
			"bytes", c.Writer.Size(),
		)
	}
}

func structuredRecovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		logger.Error("http_panic",
			"request_id", c.GetString("request_id"),
			"route", routeLabel(c.FullPath()),
			"panic_type", fmt.Sprintf("%T", recovered),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"message": "Internal server error",
			"code":    "INTERNAL_ERROR",
		})
	})
}

func routeLabel(route string) string {
	if route == "" {
		return "unmatched"
	}
	return route
}

func metricsAuthMiddleware(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedToken == "" {
			c.Next()
			return
		}
		const prefix = "Bearer "
		header := c.GetHeader("Authorization")
		provided := ""
		if strings.HasPrefix(header, prefix) {
			provided = strings.TrimPrefix(header, prefix)
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expectedToken)) != 1 {
			c.Header("WWW-Authenticate", "Bearer")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(bytes[:])
}
