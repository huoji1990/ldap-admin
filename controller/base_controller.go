package controller

import (
	"github.com/eryajf/go-ldap-admin/logic"
	"github.com/eryajf/go-ldap-admin/model/request"

	"github.com/gin-gonic/gin"
)

type BaseController struct{}

// ChangePwd 用户通过邮箱修改密码
func (m *BaseController) ChangePwd(c *gin.Context) {
	req := new(request.BaseChangePwdReq)
	Run(c, req, func() (interface{}, interface{}) {
		return logic.Base.ChangePwd(c, req)
	})
}

// Dashboard 系统首页展示数据
func (m *BaseController) Dashboard(c *gin.Context) {
	req := new(request.BaseDashboardReq)
	Run(c, req, func() (interface{}, interface{}) {
		return logic.Base.Dashboard(c, req)
	})
}

// GetPasswd 生成加密密码
func (m *BaseController) GetPasswd(c *gin.Context) {
	req := new(request.GetPasswdReq)
	Run(c, req, func() (interface{}, interface{}) {
		return logic.Base.GetPasswd(c, req)
	})
}

// GetFeishuEvent 处理飞书事件
func GetFeishuEvent(c *gin.Context) {
	json := make(map[string]interface{})
	c.BindJSON(&json)
	header := json["header"].(map[string]interface{})
	event := header["event"].(map[string]interface{})
	if header["event_type"] == "contact.user.deleted_v3" {
		logic.FeishuEventDelete(event["union_id"])
	}
}
