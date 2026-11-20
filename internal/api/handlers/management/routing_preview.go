package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
)

func (h *Handler) GetFiveHourRoutingPreview(c *gin.Context) {
	model := strings.TrimSpace(c.Query("model"))
	if model == "" || len(model) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a valid model is required"})
		return
	}
	base := thinking.ParseSuffix(model).ModelName
	providers := util.GetProviderName(base)
	if len(providers) == 0 && base != model {
		providers = util.GetProviderName(model)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, h.authManager.PreviewFiveHour(c.Request.Context(), providers, model))
}
