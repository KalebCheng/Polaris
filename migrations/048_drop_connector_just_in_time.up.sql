-- 048：删除连接器上的 JIT 重复开关，归并到账号策略 createWhen=jit（Spec D12 / FR-10）
--
-- 历史行为：JIT 建号实际读 connectors.just_in_time，账号策略里的 createWhen=jit 不生效。
-- 归并规则：
--   1) 只有连接器 JIT、且应用没有启用策略 → 补一条应用专属 JIT 策略；
--   2) 已有启用策略但建号时机为空 / manual → 改为 jit（保留连接器原来的即时建号行为）；
--   3) onCreate / onGrant 策略不动（账号会更早创建，JIT 不再必要）；
--   4) 最后删除 connectors.just_in_time。
INSERT INTO account_strategies (institution_id, application_id, name, display_name, is_enabled, mapping, policy)
SELECT DISTINCT ON (c.institution_id, c.application_id)
       c.institution_id,
       c.application_id,
       'jit-connector-' || c.application_id::text,
       'JIT · ' || COALESCE(NULLIF(a.name, ''), a.code),
       TRUE,
       '{}'::jsonb,
       '{"createWhen":"jit","revokeWhen":"disable"}'::jsonb
  FROM connectors c
  JOIN applications a ON a.id = c.application_id
 WHERE c.just_in_time = TRUE
   AND c.application_id IS NOT NULL
   AND NOT EXISTS (
       SELECT 1
         FROM account_strategies s
        WHERE s.institution_id = c.institution_id
          AND s.application_id = c.application_id
          AND s.is_enabled = TRUE
   )
 ORDER BY c.institution_id, c.application_id, c.id
ON CONFLICT (institution_id, name) DO NOTHING;

UPDATE account_strategies s
   SET policy = jsonb_set(COALESCE(s.policy, '{}'::jsonb), '{createWhen}', '"jit"', TRUE),
       updated_at = NOW()
  FROM connectors c
 WHERE c.just_in_time = TRUE
   AND c.institution_id = s.institution_id
   AND c.application_id = s.application_id
   AND s.is_enabled = TRUE
   AND COALESCE(s.policy->>'createWhen', '') IN ('', 'manual');

ALTER TABLE connectors DROP COLUMN IF EXISTS just_in_time;
