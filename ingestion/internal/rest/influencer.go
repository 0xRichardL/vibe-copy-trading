package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/domain"
	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/store"
	"github.com/gin-gonic/gin"
)

type InfluencerController struct {
	store *store.InfluencerStore
}

func NewInfluencerController(store *store.InfluencerStore) *InfluencerController {
	return &InfluencerController{store: store}
}

func (c *InfluencerController) RegisterInfluencerRoutes(rg *gin.RouterGroup) {
	rg.GET("/influencers", c.handleListInfluencers)
	rg.POST("/influencers", c.handleAddInfluencer)
}

func (c *InfluencerController) handleListInfluencers(ctx *gin.Context) {
	influencers, err := c.store.List(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, influencers)
}

func (c *InfluencerController) handleAddInfluencer(ctx *gin.Context) {
	var req struct {
		Address string `json:"address"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Address == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "address is required"})
		return
	}

	inf := domain.Influencer{Address: req.Address}

	reqCtx, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
	defer cancel()
	if err := c.store.Add(reqCtx, inf); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
