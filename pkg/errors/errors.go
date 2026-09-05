// Package errors —— 平台统一错误：消息码 + 渲染参数（BP-03 §4.2）。
//
// 硬约束：
//   - 消息码（Code）同时作为 i18n 键，对外 API/事件永不携带已渲染文本，由前端按消息码渲染；
//   - Params 仅允许低敏渲染参数，S3/S4 级数据禁止进入（BP-05 §6.3）；
//   - Cause 仅用于日志与链路排障，HTTP 响应编码时剥离（M2 编码器接线时强制）。
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// Error 平台业务错误。
type Error struct {
	Code     string            // 消息码 / i18n 键，如 "platform.tenant_context_missing"
	HTTPCode int               // HTTP 状态映射
	Params   map[string]string // 渲染参数（禁止敏感数据）
	Cause    error             // 内部原因，永不外传
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Cause }

// WithParam 附加渲染参数（链式）。
func (e *Error) WithParam(key, value string) *Error {
	if e.Params == nil {
		e.Params = make(map[string]string, 1)
	}
	e.Params[key] = value
	return e
}

// New 创建平台错误。
func New(code string, httpCode int) *Error {
	return &Error{Code: code, HTTPCode: httpCode}
}

// Wrap 包装内部错误（Cause 仅入日志，不外传）。
func Wrap(cause error, code string, httpCode int) *Error {
	return &Error{Code: code, HTTPCode: httpCode, Cause: cause}
}

// From 提取错误链中的平台错误。
func From(err error) (*Error, bool) {
	var e *Error
	if stderrors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// CodeOf 返回错误的消息码；非平台错误统一归并为 internal（防内部信息泄漏）。
func CodeOf(err error) string {
	if e, ok := From(err); ok {
		return e.Code
	}
	return "platform.internal"
}

// 常用兜底码。
var (
	ErrInternal = New("platform.internal", http.StatusInternalServerError)
)
