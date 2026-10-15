-- 027：提供商（providers）契约收敛 —— 第四阶段：改名「提供商」+ 目录 schema 驱动 + 外部 IdP 接线。
--
-- 背景（2026-10-13 代码勘察）：
--   1) category 取值大小写不一致（'OAuth'/'Email'/'Captcha'…），且前端误用小写比较；
--   2) 第三方登录 / 标准 OIDC 的 providers 行**没有任何运行时读取点**——真正生效的是
--      configs/app.yaml 的 auth.oauth.providers（启动期注册）；本迁移把信任策略与回调覆盖
--      落到提供商行上，使控制台成为唯一入口；
--   3) 9 个字段全仓无读取点（method/cert/client_id2/client_secret2/email_regex/enable_proxy/
--      http_headers/user_mapping/custom_logout_url），sub_type 与 type 语义重叠；
--   4) client_secret 明文入库（应用层改经 SecretCipher 加密，存量由 -rotate-provider-secrets 回填）。
--
-- 回滚：027_provider_catalog_contract.down.sql

-- ── 1. 类别归一化 + OIDC 类别并入 oauth（"外部身份提供方"是同一件事）─────────
UPDATE providers SET category = lower(category) WHERE category <> lower(category);
UPDATE providers SET category = 'oauth' WHERE category = 'oidc';

-- ── 2. 类型收敛：sub_type 的厂商语义并入 type（仅这三类曾用 sub_type 表达）────
UPDATE providers SET type = lower(sub_type)
 WHERE category IN ('sms', 'captcha', 'storage') AND sub_type <> '';
UPDATE providers SET type = lower(type) WHERE type <> lower(type);
-- 验证码历史子类型（recaptcha_v2/v3）统一为 recaptcha
UPDATE providers SET type = 'recaptcha' WHERE category = 'captcha' AND type IN ('recaptcha_v2', 'recaptcha_v3');
-- 空类型回落到该类别默认类型
UPDATE providers SET type = CASE category
    WHEN 'captcha' THEN 'image'
    WHEN 'storage' THEN 'local'
    WHEN 'email'   THEN 'smtp'
    WHEN 'oauth'   THEN 'oidc'
    ELSE type END
 WHERE type = '';

-- ── 3. 删除死列与冗余列（全仓无读取点，先采样留痕再删）──────────────────────
CREATE TABLE IF NOT EXISTS provider_dropped_columns_backup AS
SELECT id, method, cert, client_id2, client_secret2, email_regex, enable_proxy,
       http_headers, user_mapping, custom_logout_url, sub_type, NOW() AS snapshot_at
  FROM providers;

ALTER TABLE providers
    DROP COLUMN IF EXISTS method,
    DROP COLUMN IF EXISTS cert,
    DROP COLUMN IF EXISTS client_id2,
    DROP COLUMN IF EXISTS client_secret2,
    DROP COLUMN IF EXISTS email_regex,
    DROP COLUMN IF EXISTS enable_proxy,
    DROP COLUMN IF EXISTS http_headers,
    DROP COLUMN IF EXISTS user_mapping,
    DROP COLUMN IF EXISTS custom_logout_url,
    DROP COLUMN IF EXISTS sub_type;

-- ── 4. 外部身份提供方（SSO）信任策略 + 回调覆盖（原在 yaml 的 provisioning/redirect_uri）──
-- 只声明**运行时会真实读取**的策略字段（见 internal/service/provider/catalog.go 的 policyFields）：
-- provision_mode / default_role 在原 yaml 配置里也从未被读取，故不再引入（避免"配了不生效"）。
ALTER TABLE providers
    ADD COLUMN IF NOT EXISTS auto_create     BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS link_by_email   BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS allowed_domains TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS redirect_uri    TEXT    NOT NULL DEFAULT '';

-- 索引：运行时按 (组织, 类别) 取启用的提供商（通知/存储/验证码/登录四处读取点）
CREATE INDEX IF NOT EXISTS providers_institution_category_idx ON providers (institution_id, category);

COMMENT ON COLUMN providers.client_secret IS 'AES-GCM 密文（SecretCipher）；空串表示未配置';
