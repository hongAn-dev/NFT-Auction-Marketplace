package handler

import (
	"log/slog"
	"net/http"

	"bidding-service/internal/domain"
	"github.com/gin-gonic/gin"
)

// NFTHTTPHandler quản lý việc nhận request HTTP, gọi Usecase và trả về Response tương ứng cho Client.
type NFTHTTPHandler struct {
	usecase      domain.NFTUsecase
	metadataRepo domain.NFTMetadataRepository
}

// NewNFTHTTPHandler khởi tạo đối tượng HTTP handler nhận cả Usecase (SQL/Redis) và MetadataRepo (MongoDB).
func NewNFTHTTPHandler(uc domain.NFTUsecase, metaRepo domain.NFTMetadataRepository) *NFTHTTPHandler {
	return &NFTHTTPHandler{
		usecase:      uc,
		metadataRepo: metaRepo,
	}
}

// RegisterRoutes đăng ký các API endpoints (CRUD) vào Gin Router.
func (h *NFTHTTPHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		api.GET("/nfts", h.GetAll)
		api.GET("/nfts/:id", h.GetByID)
		api.POST("/nfts", h.Create)
		api.PUT("/nfts/:id", h.Update)
		api.DELETE("/nfts/:id", h.Delete)

		// Thêm endpoint ghi Metadata động vào MongoDB
		api.POST("/nfts/:id/metadata", h.UpsertMetadata)
	}
}

// GetAll - Lấy toàn bộ danh sách NFT
// @Summary Lấy toàn bộ danh sách NFT
// @Description Lấy toàn bộ danh sách các tác phẩm NFT đang có trong PostgreSQL
// @Tags NFTs
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/nfts [get]
func (h *NFTHTTPHandler) GetAll(c *gin.Context) {
	ctx := c.Request.Context()
	nfts, err := h.usecase.ListNFTs(ctx)
	if err != nil {
		slog.Error("Failed to list NFTs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_SERVER_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nfts,
	})
}

// GetByID - Lấy thông tin NFT theo ID hợp nhất dữ liệu Postgres và MongoDB
// @Summary Lấy thông tin chi tiết NFT
// @Description Lấy chi tiết NFT theo ID, tự động gộp dữ liệu từ Postgres (SQL) và MongoDB (NoSQL Metadata)
// @Tags NFTs
// @Accept json
// @Produce json
// @Param id path string true "NFT ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/nfts/{id} [get]
func (h *NFTHTTPHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	nft, err := h.usecase.GetNFT(ctx, id)
	if err != nil {
		if err == domain.ErrNFTNotFound {
			slog.Warn("NFT not found", "nft_id", id)
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NFT_NOT_FOUND",
					"message": "Không tìm thấy NFT với ID yêu cầu",
				},
			})
			return
		}

		slog.Error("Failed to get NFT", "nft_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_SERVER_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	// ── ĐỌC METADATA ĐỘNG TỪ MONGODB ──────────────────────────────
	// Áp dụng tính năng chịu lỗi (Fault Tolerance): Nếu MongoDB lỗi hoặc chưa có metadata,
	// ta vẫn trả về thông tin cơ bản của NFT từ Postgres bình thường, không làm sập API.
	var metadata *domain.NFTMetadata
	if h.metadataRepo != nil {
		var errMeta error
		metadata, errMeta = h.metadataRepo.GetByNFTID(ctx, id)
		if errMeta != nil {
			slog.Warn("Failed to read dynamic metadata from MongoDB", "nft_id", id, "error", errMeta)
			c.Header("X-Metadata-Error", errMeta.Error())
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"data":     nft,
		"metadata": metadata, // Hợp nhất cấu trúc SQL + NoSQL trong cùng một response!
	})
}

// Create - Tạo mới một NFT
// @Summary Tạo mới một tác phẩm NFT
// @Description Tạo mới NFT và lưu vào PostgreSQL
// @Tags NFTs
// @Accept json
// @Produce json
// @Param input body object true "NFT Create Payload"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/nfts [post]
func (h *NFTHTTPHandler) Create(c *gin.Context) {
	var input struct {
		ID          string  `json:"id" binding:"required"`
		Title       string  `json:"title" binding:"required"`
		Description string  `json:"description"`
		ImageUrl    string  `json:"image_url" binding:"required,url"`
		CreatorID   string  `json:"creator_id" binding:"required"`
		StartPrice  float64 `json:"start_price" binding:"required,gt=0"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		slog.Warn("Invalid input structure for creating NFT", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_INPUT",
				"message": "Dữ liệu đầu vào không hợp lệ",
				"details": err.Error(),
			},
		})
		return
	}

	nft := &domain.NFT{
		ID:          input.ID,
		Title:       input.Title,
		Description: input.Description,
		ImageUrl:    input.ImageUrl,
		CreatorID:   input.CreatorID,
		OwnerID:     input.CreatorID,
		StartPrice:  input.StartPrice,
	}

	ctx := c.Request.Context()
	createdNFT, err := h.usecase.CreateNFT(ctx, nft)
	if err != nil {
		slog.Error("Failed to create NFT in SQL database", "nft_id", input.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "CREATE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	slog.Info("Successfully created new NFT lot", "nft_id", createdNFT.ID, "creator", createdNFT.CreatorID)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    createdNFT,
	})
}

// Update - Cập nhật thông tin NFT
// @Summary Cập nhật thông tin NFT
// @Description Cập nhật thông tin tiêu đề, mô tả, ảnh hoặc trạng thái của NFT theo ID
// @Tags NFTs
// @Accept json
// @Produce json
// @Param id path string true "NFT ID"
// @Param input body object true "NFT Update Payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/nfts/{id} [put]
func (h *NFTHTTPHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		ImageUrl    string `json:"image_url" binding:"omitempty,url"`
		Status      string `json:"status"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		slog.Warn("Invalid input structure for updating NFT", "nft_id", id, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_INPUT",
				"message": "Dữ liệu cập nhật không hợp lệ",
				"details": err.Error(),
			},
		})
		return
	}

	nftData := &domain.NFT{
		Title:       input.Title,
		Description: input.Description,
		ImageUrl:    input.ImageUrl,
		Status:      input.Status,
	}

	ctx := c.Request.Context()
	updatedNFT, err := h.usecase.UpdateNFT(ctx, id, nftData)
	if err != nil {
		if err == domain.ErrNFTNotFound {
			slog.Warn("Failed to update: NFT not found", "nft_id", id)
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NFT_NOT_FOUND",
					"message": "Không tìm thấy NFT cần cập nhật",
				},
			})
			return
		}

		slog.Error("Failed to update NFT", "nft_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "UPDATE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	slog.Info("Successfully updated NFT", "nft_id", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    updatedNFT,
	})
}

// Delete - Xóa NFT khỏi hệ thống (đồng thời xóa cache và metadata)
// @Summary Xóa NFT khỏi hệ thống
// @Description Xóa NFT khỏi PostgreSQL, đồng thời xóa cache Redis và Metadata trong MongoDB
// @Tags NFTs
// @Accept json
// @Produce json
// @Param id path string true "NFT ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/nfts/{id} [delete]
func (h *NFTHTTPHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	err := h.usecase.DeleteNFT(ctx, id)
	if err != nil {
		if err == domain.ErrNFTNotFound {
			slog.Warn("Failed to delete: NFT not found", "nft_id", id)
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NFT_NOT_FOUND",
					"message": "Không tìm thấy NFT để xóa",
				},
			})
			return
		}

		slog.Error("Failed to delete NFT", "nft_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "DELETE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	// Xóa kèm metadata trong MongoDB
	if h.metadataRepo != nil {
		_ = h.metadataRepo.Delete(ctx, id)
	}

	slog.Info("Successfully deleted NFT lot", "nft_id", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Xóa NFT và toàn bộ thông tin đi kèm thành công!",
	})
}

// UpsertMetadata - Tạo hoặc cập nhật Metadata phi cấu trúc vào MongoDB
// @Summary Thêm hoặc cập nhật Metadata động (MongoDB)
// @Description Lưu các thuộc tính phi cấu trúc (generative traits, IPFS hash, độ hiếm) vào MongoDB kết nối với NFT Postgres
// @Tags NFTs
// @Accept json
// @Produce json
// @Param id path string true "NFT ID"
// @Param input body object true "Metadata Payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/nfts/{id}/metadata [post]
func (h *NFTHTTPHandler) UpsertMetadata(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Rarity     string                 `json:"rarity" binding:"required"`
		Attributes map[string]interface{} `json:"attributes" binding:"required"`
		IPFSHash   string                 `json:"ipfs_hash" binding:"required"`
		CreatedBy  string                 `json:"created_by" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		slog.Warn("Invalid metadata input details", "nft_id", id, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_INPUT",
				"message": "Dữ liệu metadata không hợp lệ",
				"details": err.Error(),
			},
		})
		return
	}

	// 1. Kiểm tra xem NFT có tồn tại trong Postgres không trước khi ghi metadata
	ctx := c.Request.Context()
	_, err := h.usecase.GetNFT(ctx, id)
	if err != nil {
		if err == domain.ErrNFTNotFound {
			slog.Warn("Failed to upsert metadata: parent NFT not found in PostgreSQL", "nft_id", id)
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NFT_NOT_FOUND",
					"message": "Không thể thêm metadata cho NFT không tồn tại trong hệ thống chính",
				},
			})
			return
		}
		
		slog.Error("Failed to find parent NFT for metadata upsert", "nft_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 2. Chuẩn bị thực thể metadata lưu vào MongoDB
	metadata := &domain.NFTMetadata{
		NFTID:      id,
		Rarity:     input.Rarity,
		Attributes: input.Attributes,
		CreatedBy:  input.CreatedBy,
		IPFSHash:   input.IPFSHash,
	}

	if h.metadataRepo == nil {
		slog.Warn("MongoDB metadata service is not available", "nft_id", id)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "Dịch vụ lưu trữ MongoDB hiện không khả dụng",
		})
		return
	}

	err = h.metadataRepo.Upsert(ctx, metadata)
	if err != nil {
		slog.Error("Failed to upsert dynamic metadata to MongoDB", "nft_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Lỗi khi lưu trữ vào MongoDB: " + err.Error(),
		})
		return
	}

	slog.Info("Successfully upserted dynamic metadata in MongoDB NoSQL", "nft_id", id)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Cập nhật Metadata vào MongoDB thành công!",
		"data":    metadata,
	})
}
