package response

import "github.com/gin-gonic/gin"

// 统一响应包：{"code":0,"message":"ok","data":...}
// 错误 code 四位：4000 参数 / 4401 未认证 / 4403 无权限 / 4404 不存在 / 4500 上传链路 / 5000 服务器

func OK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": data})
}

func Error(c *gin.Context, status int, code int, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "data": nil})
}

func BadRequest(c *gin.Context, message string) {
	Error(c, 400, 4000, message)
}

func Unauthorized(c *gin.Context, message string) {
	Error(c, 401, 4401, message)
}

func Forbidden(c *gin.Context, message string) {
	Error(c, 403, 4403, message)
}

func NotFound(c *gin.Context, message string) {
	Error(c, 404, 4404, message)
}

func ServerError(c *gin.Context, message string) {
	Error(c, 500, 5000, message)
}
