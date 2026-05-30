package logger

import (
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// InitLogger khởi tạo structured logger (slog) và thiết lập làm logger mặc định của hệ thống.
func InitLogger() *slog.Logger {
	var handler slog.Handler
	env := os.Getenv("APP_ENV")

	if env == "production" {
		// Log dạng JSON trong môi trường production/docker để phục vụ log aggregators (ELK, Loki)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		// Log dạng Text có màu sắc/dễ nhìn trong môi trường local development
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("🔒 Structured Logging (slog) đã được khởi tạo thành công!", "env", env)
	return logger
}

// GinLoggerMiddleware là Gin middleware tự động ghi log mọi HTTP request dưới dạng structured log.
func GinLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// Ghi nhận structured log
		attributes := []any{
			slog.String("method", method),
			slog.String("path", path),
			slog.String("ip", clientIP),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("latency_human", latency.String()),
		}

		if query != "" {
			attributes = append(attributes, slog.String("query", query))
		}

		// Nếu có lỗi do Gin middleware ghi nhận, đưa vào log attributes
		if len(c.Errors) > 0 {
			attributes = append(attributes, slog.String("error", c.Errors.String()))
			slog.Error("HTTP Request Error", attributes...)
		} else if status >= 400 && status < 500 {
			slog.Warn("HTTP Request Warning", attributes...)
		} else if status >= 500 {
			slog.Error("HTTP Server Error", attributes...)
		} else {
			slog.Info("HTTP Request Processed", attributes...)
		}
	}
}
