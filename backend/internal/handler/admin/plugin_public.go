package admin

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *PluginHandler) ListPublic(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	plugins, err := h.manager.ListPublicWebsitePlugins(c.Request.Context())
	if err != nil {
		response.InternalError(c, "公开网站插件暂时不可用")
		return
	}
	response.Success(c, plugins)
}

// ServePublicAsset serves only an enabled website plugin's declared public/
// assets. It cannot serve its configuration UI or its original package.
func (h *PluginHandler) ServePublicAsset(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, ok := pluginIDParam(c)
	if !ok {
		return
	}
	data, logicalPath, err := h.manager.ReadPublicWebsiteAsset(c.Request.Context(), id, c.Param("hash"), strings.TrimPrefix(c.Param("path"), "/"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(logicalPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Header("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
	c.Data(http.StatusOK, contentType, data)
}
