package api

import (
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/gin-gonic/gin"
)

func (s *Server) accountWorkbenchSMSReceipts(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	result, err := service.SMSReceiptsScoped(c.Request.Context(), owner, accountworkbench.ExportScope(c.Query("scope")))
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) accountWorkbenchInspectSMSReceipt(c *gin.Context) {
	service, owner, ok := s.workbenchService(c)
	if !ok {
		return
	}
	var input accountworkbench.OAuthSMSInput
	if err := bindRequestJSON(c, &input); err != nil {
		writeError(c, 422, "原短信订单查询参数无效")
		return
	}
	result, err := service.InspectSMSReceiptScoped(c.Request.Context(), owner, c.Param("id"), accountworkbench.ExportScope(c.Query("scope")), input)
	if err != nil {
		s.browserError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
