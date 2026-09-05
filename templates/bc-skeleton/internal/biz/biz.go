// Package biz —— 领域层：聚合、领域服务与用例（DDD 分层，BP-02 §2.2）。
//
// 硬约束（ADR-0004，CI depguard 机器强制）：
//   - 禁止 import kratos 及任何传输/框架包（net/http、google.golang.org/grpc）；
//   - 仓储/网关等端口（接口）在本层定义，由 data 层实现（依赖倒置）；
//   - 仅依赖标准库与本模块内部包；
//   - 事务边界与领域事件（经 outbox 发出，ADR-0005）在本层编排。
package biz
