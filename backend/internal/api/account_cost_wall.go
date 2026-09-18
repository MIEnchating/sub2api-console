package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) setAccountIgnoreCostWall(c *gin.Context) {
	accountID := strings.TrimSpace(c.Param("account_id"))
	if !positiveNumericID(accountID) {
		writeError(c, http.StatusUnprocessableEntity, "账号必须使用有效的稳定 ID")
		return
	}
	payload, err := decodeRequestObject(c)
	enabled, valid := payload["ignore_cost_wall"].(bool)
	if err != nil || len(payload) != 1 || !valid {
		writeError(c, http.StatusUnprocessableEntity, "参数必须只包含布尔值 ignore_cost_wall")
		return
	}
	if _, ok := s.accountMutationPreflight(c, accountID, false); !ok {
		return
	}
	actor, err := s.requestActor(c)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "控制台会话读取失败")
		return
	}
	if err := s.business.SetAccountIgnoreCostWall(c.Request.Context(), accountID, enabled, actor); err != nil {
		writeError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ignore_cost_wall": enabled})
}
