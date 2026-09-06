// Package service —— BC-P1 接口层（步 17）：api/ 契约 → 领域用例的适配。
//
// 职责边界：仅做协议映射（proto ↔ biz）与错误编码，不含业务规则；
// 平台错误 → kratos 传输错误：Code=HTTP 状态、Reason=消息码（兼 i18n 键）、Params 入 Metadata
//（gRPC 经 GRPCStatus 自动携带 ErrorInfo 明细；HTTP 由 server 层编码为 PlatformError JSON）。
package service

import (
	"strconv"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/jsl-aiot/platform/api/common/v1"
	tenantv1 "github.com/jsl-aiot/platform/api/tenant/v1"
	"github.com/jsl-aiot/platform/pkg/errors"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// protoError 领域错误 → kratos 传输错误；非平台错误原样上抛（由框架归并为 500）。
func protoError(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := errors.From(err); ok {
		ke := kratoserrors.New(e.HTTPCode, e.Code, "") // Message 留空：永不携带已渲染文本
		if len(e.Params) > 0 {
			ke = ke.WithMetadata(e.Params)
		}
		return ke
	}
	return err
}

// auditFields 领域审计 → 契约审计字段。
func auditFields(a biz.Audit) *commonv1.AuditFields {
	return &commonv1.AuditFields{
		CreatedBy: a.CreatedBy,
		CreatedAt: timestamppb.New(a.CreatedAt),
		UpdatedBy: a.UpdatedBy,
		UpdatedAt: timestamppb.New(a.UpdatedAt),
		Version:   a.Version,
	}
}

// isolationFromProto 隔离级别：UNSPECIFIED 返回空串（由套餐默认值决定）。
func isolationFromProto(v tenantv1.IsolationLevel) biz.IsolationLevel {
	switch v {
	case tenantv1.IsolationLevel_ISOLATION_LEVEL_T1_DEDICATED:
		return biz.IsolationT1
	case tenantv1.IsolationLevel_ISOLATION_LEVEL_T2_SCHEMA:
		return biz.IsolationT2
	case tenantv1.IsolationLevel_ISOLATION_LEVEL_T3_ROW:
		return biz.IsolationT3
	default:
		return ""
	}
}

// isolationToProto 隔离级别映射（未知归并 UNSPECIFIED）。
func isolationToProto(v biz.IsolationLevel) tenantv1.IsolationLevel {
	switch v {
	case biz.IsolationT1:
		return tenantv1.IsolationLevel_ISOLATION_LEVEL_T1_DEDICATED
	case biz.IsolationT2:
		return tenantv1.IsolationLevel_ISOLATION_LEVEL_T2_SCHEMA
	case biz.IsolationT3:
		return tenantv1.IsolationLevel_ISOLATION_LEVEL_T3_ROW
	default:
		return tenantv1.IsolationLevel_ISOLATION_LEVEL_UNSPECIFIED
	}
}

// statusToProto 租户状态映射。
func statusToProto(v biz.TenantStatus) tenantv1.TenantStatus {
	switch v {
	case biz.TenantStatusActive:
		return tenantv1.TenantStatus_TENANT_STATUS_ACTIVE
	case biz.TenantStatusSuspended:
		return tenantv1.TenantStatus_TENANT_STATUS_SUSPENDED
	case biz.TenantStatusDeactivated:
		return tenantv1.TenantStatus_TENANT_STATUS_DEACTIVATED
	default:
		return tenantv1.TenantStatus_TENANT_STATUS_UNSPECIFIED
	}
}

// 分页：游标为十进制偏移量（内存切片分页；表规模增长后由 data 层游标查询替换）。
const (
	defaultPageSize = 100
	maxPageSize     = 200
)

// decodePageToken 解析分页游标（非法或缺省从 0 开始）。
func decodePageToken(tok string) int {
	n, err := strconv.Atoi(tok)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// pageSlice 对全量结果切片；返回本页与下一页游标（空串表示无更多数据）。
func pageSlice[T any](all []T, offset, pageSize int) ([]T, string) {
	if offset > len(all) {
		offset = len(all)
	}
	end := offset + pageSize
	if end > len(all) {
		end = len(all)
	}
	next := ""
	if offset+pageSize < len(all) {
		next = strconv.Itoa(offset + pageSize)
	}
	return all[offset:end], next
}
