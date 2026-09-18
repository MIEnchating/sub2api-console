package api

import (
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/gin-gonic/gin"
)

func (s *Server) navigationPreferences(c *gin.Context) {
	result, err := s.private.NavigationPreferences(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "菜单设置读取失败，请重试")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) saveNavigationPreferences(c *gin.Context) {
	var input configstore.NavigationPreferences
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, http.StatusBadRequest, "菜单设置格式无效")
		return
	}
	result, err := s.private.SaveNavigationPreferences(c.Request.Context(), input)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, configstore.ErrNavigationConflict) {
			status = http.StatusConflict
		}
		writeError(c, status, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
