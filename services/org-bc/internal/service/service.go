// Package service —— 接口层：proto 生成接口的实现（壳层）。
//
// 职责边界：
//   - 仅做 DTO ↔ 领域对象转换与用例编排，禁止业务逻辑；
//   - 服务实现经 kratos service 注册到 HTTP/gRPC 服务器（internal/server）；
//   - 错误统一使用 pkg/errors 消息码封装（M1 接入，BP-03 §4.2 i18n）。
package service
