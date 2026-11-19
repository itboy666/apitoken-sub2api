package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWebsitePluginLifecycleRetainsNativeAdminAuthAndStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin", func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) })
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{Plugin: adminhandler.NewPluginHandler(nil)}}
	registerPluginRoutes(admin, handlers, stepUp)
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/api/v1/admin/plugins/upload"},
		{"POST", "/api/v1/admin/plugins/7/enable"},
		{"POST", "/api/v1/admin/plugins/7/disable"},
		{"DELETE", "/api/v1/admin/plugins/7"},
		{"PUT", "/api/v1/admin/plugins/7/config"},
		{"POST", "/api/v1/admin/plugins/7/config/recover"},
		{"POST", "/api/v1/admin/plugins/7/test"},
	} {
		for _, authenticated := range []bool{false, true} {
			request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			want := http.StatusUnauthorized
			if authenticated {
				request.Header.Set("Authorization", "Bearer test")
				want = http.StatusForbidden
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, want, recorder.Code, endpoint.method+" "+endpoint.path)
		}
	}
}

func TestLegacySupportWidgetSettingsRoutesAreAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	registerSettingsRoutes(router.Group("/api/v1/admin"), handlers, middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }))
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/admin/settings/support-widget", nil))
		require.Equal(t, http.StatusNotFound, recorder.Code)
	}
}
