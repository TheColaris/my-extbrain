package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// ApiError 统一错误结构：{"error":{"code","message"}}，message 必须可执行（说清缺什么/怎么办）。
type ApiError struct {
	Code    string
	Message string
	Status  int
}

func (e *ApiError) Error() string { return e.Code + ": " + e.Message }

func errBadRequest(code, msg string) *ApiError {
	return &ApiError{Code: code, Message: msg, Status: 400}
}
func errUnauthorized(msg string) *ApiError {
	return &ApiError{Code: "unauthorized", Message: msg, Status: 401}
}
func errForbidden(msg string) *ApiError {
	return &ApiError{Code: "forbidden", Message: msg, Status: 403}
}
func errNotFound(msg string) *ApiError {
	return &ApiError{Code: "not_found", Message: msg, Status: 404}
}
func errConflict(code, msg string) *ApiError { return &ApiError{Code: code, Message: msg, Status: 409} }
func errTooMany(msg string) *ApiError {
	return &ApiError{Code: "rate_limited", Message: msg, Status: 429}
}
func errInternal(msg string) *ApiError { return &ApiError{Code: "internal", Message: msg, Status: 500} }

// fail 写出统一错误 JSON
func fail(c *gin.Context, e *ApiError) {
	c.AbortWithStatusJSON(e.Status, gin.H{"error": gin.H{"code": e.Code, "message": e.Message}})
}

// paramID 解析 :id 路由参数，失败已写出响应（返回 0, err）
func paramID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return 0, err
	}
	return id, nil
}

func atoiDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def, err
	}
	return n, nil
}
