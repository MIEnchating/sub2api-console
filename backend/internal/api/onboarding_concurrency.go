package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (s *Server) previewOnboardingConcurrency(c *gin.Context) {
	service, ok := s.onboarding.(interface {
		PreviewConcurrency(context.Context, []onboarding.Request) ([]onboarding.ConcurrencyAllocation, error)
	})
	if !ok {
		writeError(c, http.StatusServiceUnavailable, "账号并发分配服务尚未就绪")
		return
	}
	payload, err := decodeRequestObject(c)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, "并发预览参数必须是 JSON 对象")
		return
	}
	requests, err := parseOnboardingBatchRequests(payload)
	if err != nil {
		writeError(c, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := service.PreviewConcurrency(c.Request.Context(), requests)
	if err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": result})
}
