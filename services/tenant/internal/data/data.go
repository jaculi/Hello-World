// Package data —— BC-P1 数据访问层。
//
// 本期（M3/步 15-16）实现：
//   - tenants / organizations / sites / plans / subscriptions 五表仓储（BP-03 §3/§4.1）
//   - 全部经 pkg/dataaccess 基类：事务内 SET LOCAL app.tenant_id 绑定 RLS（BP-03 §3.2）
//   - 租户隔离级别元数据的路由表登记（初版 T3 单库 RLS，T1/T2 接口预留）
package data
