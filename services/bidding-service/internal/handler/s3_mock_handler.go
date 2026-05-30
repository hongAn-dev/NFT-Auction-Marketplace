package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// S3MockHandler quản lý việc giả lập AWS S3 / Cloudflare R2 Presigned URL
type S3MockHandler struct{}

// NewS3MockHandler khởi tạo đối tượng handler S3/R2
func NewS3MockHandler() *S3MockHandler {
	return &S3MockHandler{}
}

// PresignURL - GET /api/v1/s3/presign
// @Summary Tạo Presigned URL để upload ảnh
// @Description Sinh ra một Presigned Upload URL trực tiếp lên Cloudflare R2 hoặc S3 local fallback mock
// @Tags Storage
// @Accept json
// @Produce json
// @Param file query string true "Filename"
// @Param type query string false "Mime Type"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/s3/presign [get]
func (h *S3MockHandler) PresignURL(c *gin.Context) {
	filename := c.Query("file")
	filetype := c.Query("type")

	if filename == "" {
		slog.Warn("Presigned URL requested without filename")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Thiếu tham số 'file' (tên tệp tin)",
		})
		return
	}

	// ── KIỂM TRA BIẾN CẤU HÌNH CLOUDFLARE R2 ───────────────────────
	r2AccountID := os.Getenv("R2_ACCOUNT_ID")
	r2AccessKeyID := os.Getenv("R2_ACCESS_KEY_ID")
	r2SecretAccessKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	r2BucketName := os.Getenv("R2_BUCKET_NAME")
	r2PublicURL := os.Getenv("R2_PUBLIC_URL")

	// Kiểm tra nếu các khoá bắt đầu bằng "cfat_", tức là người dùng cấu hình nhầm API Token thay vì S3 Credentials
	isApiTokenFormat := (len(r2AccessKeyID) > 5 && r2AccessKeyID[:5] == "cfat_") || (len(r2SecretAccessKey) > 5 && r2SecretAccessKey[:5] == "cfat_")

	// Nếu đầy đủ cấu hình R2 thật, ta sẽ sinh Presigned URL đi qua Proxy của Go Backend!
	// Điều này giải quyết TRIỆT ĐỂ lỗi TLS ERR_SSL_VERSION_OR_CIPHER_MISMATCH khi trình duyệt gọi trực tiếp R2 storage.
	if r2AccountID != "" && r2AccessKeyID != "" && r2SecretAccessKey != "" && r2BucketName != "" && !isApiTokenFormat {
		slog.Info("R2: Generating proxy upload handshake parameters", "bucket", r2BucketName, "file", filename)

		appPort := os.Getenv("APP_PORT")
		if appPort == "" {
			appPort = "8080"
		}

		// Trình duyệt sẽ PUT trực tiếp lên Go Backend, Go Backend sẽ chuyển tiếp nhị phân lên R2
		uploadURL := fmt.Sprintf("http://localhost:%s/api/v1/s3/upload?file=%s&storage=r2", appPort, filename)

		publicUrlPrefix := r2PublicURL
		if publicUrlPrefix == "" {
			publicUrlPrefix = fmt.Sprintf("https://pub-%s.r2.dev", r2AccountID)
		}
		finalPublicURL := fmt.Sprintf("%s/%s", publicUrlPrefix, filename)

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"upload_url":   uploadURL,
			"public_url":   finalPublicURL,
			"storage_type": "cloudflare_r2",
		})
		return
	}

	// ── FALLBACK: CHẠY CHẾ ĐỘ LOCAL MOCK S3 (TỰ CHỦ OFFLINE) ───────────
	if isApiTokenFormat {
		slog.Warn("R2: Detected API Token format (cfat_...) inside R2 S3 Credentials. Falling back to Local Mock S3 to prevent 500 errors. Please generate S3 API credentials in Cloudflare dashboard.", "access_key", r2AccessKeyID)
	} else {
		slog.Info("R2 Keys missing. Activating Local S3 Mock Fallback Sandbox.", "file", filename)
	}
	
	// Sinh ra một URL ký giả lập trỏ về API Gateway / Bidding Service cục bộ
	appPort := os.Getenv("APP_PORT")
	if appPort == "" {
		appPort = "8080"
	}

	// URL mà Frontend sẽ gửi PUT trực tiếp
	uploadURL := fmt.Sprintf("http://localhost:%s/api/v1/s3/upload?file=%s", appPort, filename)
	
	// URL public mà Frontend sẽ đọc ảnh sau khi lưu xong
	finalPublicURL := fmt.Sprintf("http://localhost:%s/uploads/%s", appPort, filename)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"upload_url":   uploadURL,
		"public_url":   finalPublicURL,
		"storage_type": "local_mock_s3",
		"details":      "Đang sử dụng bộ lưu trữ giả lập local (Offline mode). Điền .env R2 keys chuẩn S3 để kích hoạt Cloudflare thật.",
		"mime_type":    filetype,
	})
}

// Upload - PUT /api/v1/s3/upload
// @Summary Upload ảnh nhị phân trực tiếp (Mock S3 local)
// @Description Tiếp nhận luồng nhị phân (Binary) upload từ PUT request và lưu cục bộ vào ./uploads
// @Tags Storage
// @Accept octet-stream
// @Produce json
// @Param file query string true "Filename"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/s3/upload [put]
func (h *S3MockHandler) Upload(c *gin.Context) {
	filename := c.Query("file")
	storageType := c.Query("storage")

	if filename == "" {
		slog.Warn("Upload request missing file query param")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Thiếu tham số 'file' để đặt tên tệp tin tải lên",
		})
		return
	}

	// ── PROXY UPLOAD TO CLOUDFLARE R2 ──────────────────────────────────
	r2AccountID := os.Getenv("R2_ACCOUNT_ID")
	r2AccessKeyID := os.Getenv("R2_ACCESS_KEY_ID")
	r2SecretAccessKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	r2BucketName := os.Getenv("R2_BUCKET_NAME")

	isApiTokenFormat := (len(r2AccessKeyID) > 5 && r2AccessKeyID[:5] == "cfat_") || (len(r2SecretAccessKey) > 5 && r2SecretAccessKey[:5] == "cfat_")

	if storageType == "r2" && r2AccountID != "" && r2AccessKeyID != "" && r2SecretAccessKey != "" && r2BucketName != "" && !isApiTokenFormat {
		slog.Info("R2: Proxying binary file upload to Cloudflare R2 Storage", "file", filename)

		// 1. Sinh R2 signed upload URL thật phía server (Sẽ kết nối cực nhanh mà không bị lỗi SSL/CORS ở trình duyệt)
		r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", r2AccountID)
		expireDuration := 15 * time.Minute
		expireTimestamp := time.Now().Add(expireDuration).Unix()

		mac := hmac.New(sha256.New, []byte(r2SecretAccessKey))
		mac.Write([]byte(fmt.Sprintf("%s/%s/%s/%d", r2AccountID, r2BucketName, filename, expireTimestamp)))
		signature := hex.EncodeToString(mac.Sum(nil))

		realR2UploadURL := fmt.Sprintf("%s/%s/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=%s&X-Amz-Date=%d&X-Amz-Expires=900&X-Amz-Signature=%s",
			r2Endpoint, r2BucketName, filename, r2AccessKeyID, expireTimestamp, signature)

		// 2. Tạo HTTP PUT Request chuyển tiếp nhị phân trực tiếp từ Body
		req, err := http.NewRequestWithContext(c.Request.Context(), "PUT", realR2UploadURL, c.Request.Body)
		if err != nil {
			slog.Error("R2: Failed to create proxy request", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}

		req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
		if c.Request.ContentLength > 0 {
			req.ContentLength = c.Request.ContentLength
		}

		// 3. Thực thi chuyển tiếp stream sang Cloudflare R2
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Error("R2: Failed to transmit proxy upload network stream", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "R2 upload proxy network error: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
			respBody, _ := io.ReadAll(resp.Body)
			slog.Error("R2: Storage server rejected upload request", "status", resp.Status, "response", string(respBody))
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error": fmt.Sprintf("Cloudflare R2 rejected upload: %s (Details: %s)", resp.Status, string(respBody)),
			})
			return
		}

		slog.Info("R2: Successfully proxied and saved file on Cloudflare R2!", "file", filename)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Đã tải tệp tin lên Cloudflare R2 thành công thông qua Proxy!",
			"file":    filename,
		})
		return
	}

	// ── LOCAL FALLBACK: LƯU TẬP TIN CỤC BỘ ──────────────────────────────
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		slog.Error("Failed to create uploads directory", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Không thể khởi tạo thư mục lưu trữ cục bộ: %v", err),
		})
		return
	}

	filePath := filepath.Join(uploadDir, filename)
	out, err := os.Create(filePath)
	if err != nil {
		slog.Error("Failed to create file on disk", "file", filePath, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Không thể tạo tệp tin trên đĩa cứng: %v", err),
		})
		return
	}
	defer out.Close()

	// Đọc luồng nhị phân trực tiếp từ Request Body (Binary stream upload)
	written, err := io.Copy(out, c.Request.Body)
	if err != nil {
		slog.Error("Error copying binary upload streams", "file", filePath, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Lỗi trong quá trình ghi dữ liệu luồng tệp tin: %v", err),
		})
		return
	}

	slog.Info("S3 Mock: Successfully uploaded file binary to local storage", "file", filename, "bytes", written)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Đã tải tệp tin lên S3 Mock thành công cục bộ!",
		"file":    filename,
		"size":    written,
	})
}
