// BC-P1 领域消息码（兼 i18n 键，BP-03 §4.2）。
package biz

// 领域消息码。
const (
	CodeTenantNotFound          = "platform.tenant.not_found"
	CodeTenantNameTaken         = "platform.tenant.name_taken"
	CodeTenantNameInvalid       = "platform.tenant.name_invalid"
	CodeTenantInvalidTransition = "platform.tenant.invalid_transition"
	CodeTenantCrossDenied       = "platform.tenant.cross_tenant_denied"
	CodeTenantDeactivated       = "platform.tenant.deactivated"
	CodeTenantIsolationUnsupported = "platform.tenant.isolation_unsupported"
	CodePlanNotFound            = "platform.plan.not_found"
	CodePlanInactive            = "platform.plan.inactive"
	CodeOrgNotFound             = "platform.org.not_found"
	CodeOrgNameTaken            = "platform.org.name_taken"
	CodeOrgNameInvalid          = "platform.org.name_invalid"
	CodeSiteNotFound            = "platform.site.not_found"
	CodeSiteNameTaken           = "platform.site.name_taken"
	CodeSiteNameInvalid         = "platform.site.name_invalid"
	CodeSiteCycle               = "platform.site.cycle"
	CodeVersionConflict         = "platform.version_conflict"
)

// HTTP 状态映射（平台错误 → 传输状态）。
const (
	statusBadRequest    = 400
	statusNotFound      = 404
	statusConflict      = 409
	statusUnprocessable = 422
)
