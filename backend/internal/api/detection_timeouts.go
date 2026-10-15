package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/gin-gonic/gin"
)

type detectionTimeoutService interface {
	DetectionTimeout(string) (modelcheck.DetectionTimeout, error)
	SaveDetectionTimeout(context.Context, string, modelcheck.DetectionTimeout, string) (modelcheck.DetectionTimeout, error)
}

func (s *Server) detectionTimeout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := s.modelChecks.(detectionTimeoutService)
	if !ok {
		writeError(c, 503, "检测超时设置尚未就绪")
		return
	}
	var value modelcheck.DetectionTimeout
	var err error
	if c.Request.Method == http.MethodGet {
		value, err = service.DetectionTimeout(c.Param("mode"))
	} else {
		if bindRequestJSON(c, &value) != nil {
			writeError(c, 422, modelcheck.ErrDetectionTimeout.Error())
			return
		}
		actor, actorErr := s.requestActor(c)
		if actorErr != nil {
			writeError(c, 500, "控制台会话读取失败")
			return
		}
		value, err = service.SaveDetectionTimeout(c.Request.Context(), c.Param("mode"), value, actor)
	}
	if err != nil {
		switch {
		case errors.Is(err, modelcheck.ErrDetectionTimeout):
			writeError(c, 422, err.Error())
		case errors.Is(err, modelcheck.ErrDetectionTimeoutConflict):
			writeError(c, 409, err.Error())
		default:
			writeError(c, 500, "检测超时设置操作失败，请重试")
		}
		return
	}
	c.JSON(200, value)
}
