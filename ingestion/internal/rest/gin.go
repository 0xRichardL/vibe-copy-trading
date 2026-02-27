package rest

import (
	"net/http"

	"github.com/0xRichardL/vibe-copy-trading/ingestion/internal/config"
	"github.com/gin-gonic/gin"
)

func NewServer(cfg config.Config) (*gin.Engine, *http.Server) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: r,
	}
	return r, srv
}
