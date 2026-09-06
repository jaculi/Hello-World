-- BC-P1 初始 Schema（BP-03 §3/§4.1）。
-- 约定：主键 row_id（ULID）；审计四件套（created_by/at, updated_by/at）+ 乐观锁 version；软删 deleted_at。
-- RLS：租户表全量启用 FORCE ROW LEVEL SECURITY，策略绑定会话变量 app.tenant_id
--（事务内 SET LOCAL，pkg/dataaccess.TenantTx 注入；未绑定即不可见/不可写，fail-closed）。

BEGIN;

-- 迁移台账（非租户数据）
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

-- 套餐目录（平台全局读模型，非租户数据，不启用 RLS；目录运营随 BC-P3）
CREATE TABLE IF NOT EXISTS plans (
    plan_id           text PRIMARY KEY,
    name              text NOT NULL,
    default_isolation text NOT NULL,
    active            boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- 租户聚合根（行级策略以 row_id 对齐 app.tenant_id）
CREATE TABLE IF NOT EXISTS tenants (
    row_id       text PRIMARY KEY,
    name         text NOT NULL,
    display_name text NOT NULL,
    status       text NOT NULL,
    isolation    text NOT NULL,
    plan_id      text NOT NULL REFERENCES plans (plan_id),
    home_region  text NOT NULL,
    created_by   text NOT NULL,
    created_at   timestamptz NOT NULL,
    updated_by   text NOT NULL,
    updated_at   timestamptz NOT NULL,
    version      bigint NOT NULL,
    deleted_at   timestamptz,
    CONSTRAINT uq_tenants_name UNIQUE (name)
);

-- 默认套餐订阅（开通即建立；一租户一条，计费细化随 BC-P3）
CREATE TABLE IF NOT EXISTS subscriptions (
    row_id     text PRIMARY KEY,
    tenant_id  text NOT NULL REFERENCES tenants (row_id),
    plan_id    text NOT NULL REFERENCES plans (plan_id),
    status     text NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_by text NOT NULL,
    updated_at timestamptz NOT NULL,
    version    bigint NOT NULL,
    deleted_at timestamptz,
    CONSTRAINT uq_subscriptions_tenant UNIQUE (tenant_id)
);

-- 行业模块装配记录（BP-02 §6 订阅即装配）
CREATE TABLE IF NOT EXISTS industry_assemblies (
    tenant_id      text NOT NULL REFERENCES tenants (row_id),
    module_id      text NOT NULL,
    module_version text NOT NULL,
    enabled        boolean NOT NULL,
    created_by     text NOT NULL,
    created_at     timestamptz NOT NULL,
    updated_by     text NOT NULL,
    updated_at     timestamptz NOT NULL,
    version        bigint NOT NULL,
    PRIMARY KEY (tenant_id, module_id)
);

-- 组织树（租户根组织由开通时初始化；tenant_id RLS 隔离）
CREATE TABLE IF NOT EXISTS organizations (
    row_id        text PRIMARY KEY,
    tenant_id     text NOT NULL REFERENCES tenants (row_id),
    parent_org_id text REFERENCES organizations (row_id),
    name          text NOT NULL,
    display_name  text NOT NULL,
    created_by    text NOT NULL,
    created_at    timestamptz NOT NULL,
    updated_by    text NOT NULL,
    updated_at    timestamptz NOT NULL,
    version       bigint NOT NULL,
    deleted_at    timestamptz,
    CONSTRAINT uq_orgs_tenant_name UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_orgs_parent ON organizations (parent_org_id);

-- 站点树（行业中性的核心空间概念，BP-01 §2.2；tenant_id RLS 隔离）
CREATE TABLE IF NOT EXISTS sites (
    row_id         text PRIMARY KEY,
    tenant_id      text NOT NULL REFERENCES tenants (row_id),
    parent_site_id text REFERENCES sites (row_id),
    name           text NOT NULL,
    display_name   text NOT NULL,
    site_type      text NOT NULL,
    created_by     text NOT NULL,
    created_at     timestamptz NOT NULL,
    updated_by     text NOT NULL,
    updated_at     timestamptz NOT NULL,
    version        bigint NOT NULL,
    deleted_at     timestamptz,
    CONSTRAINT uq_sites_tenant_name UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_sites_parent ON sites (parent_site_id);

-- 事务性发件箱（ADR-0005：事件与业务同事务写入；relay 投递 Kafka 随步 18）
CREATE TABLE IF NOT EXISTS outbox (
    event_id     text PRIMARY KEY,           -- 事件信封 event_id（ULID，幂等键）
    event_type   text NOT NULL,
    tenant_id    text NOT NULL,              -- 所有权链冗余，relay 路由/监控用
    payload      jsonb NOT NULL,             -- EventEnvelope 完整信封
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox (created_at) WHERE published_at IS NULL;

-- ---------- RLS 策略（BP-03 §3.2） ----------

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY p_tenant_isolation ON tenants
    USING (row_id = current_setting('app.tenant_id', true))
    WITH CHECK (row_id = current_setting('app.tenant_id', true));

ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions FORCE ROW LEVEL SECURITY;
CREATE POLICY p_tenant_isolation ON subscriptions
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE industry_assemblies ENABLE ROW LEVEL SECURITY;
ALTER TABLE industry_assemblies FORCE ROW LEVEL SECURITY;
CREATE POLICY p_tenant_isolation ON industry_assemblies
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
CREATE POLICY p_tenant_isolation ON organizations
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

ALTER TABLE sites ENABLE ROW LEVEL SECURITY;
ALTER TABLE sites FORCE ROW LEVEL SECURITY;
CREATE POLICY p_tenant_isolation ON sites
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- ---------- 种子数据：默认套餐目录 ----------

INSERT INTO plans (plan_id, name, default_isolation, active)
VALUES
    ('plan-free', 'Free', 't3_row', true),
    ('plan-pro',  'Pro',  't3_row', true)
ON CONFLICT (plan_id) DO NOTHING;

INSERT INTO schema_migrations (version) VALUES ('0001_init')
ON CONFLICT (version) DO NOTHING;

COMMIT;
