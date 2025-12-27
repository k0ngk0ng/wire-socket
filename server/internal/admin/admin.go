package admin

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFiles embed.FS

// SetupRoutes adds the admin UI routes to the Gin engine
func SetupRoutes(engine *gin.Engine) {
	// Serve static files from embedded filesystem
	staticFS, _ := fs.Sub(staticFiles, "static")

	// Serve index.html at /admin
	engine.GET("/admin", func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to load admin page")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// Serve other static files at /admin/*
	engine.StaticFS("/admin/static", http.FS(staticFS))
}
