-- 048 回滚：恢复 connectors.just_in_time，并从 JIT 策略回填。
ALTER TABLE connectors ADD COLUMN IF NOT EXISTS just_in_time BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE connectors c
   SET just_in_time = TRUE
  FROM account_strategies s
 WHERE s.institution_id = c.institution_id
   AND s.application_id = c.application_id
   AND s.is_enabled = TRUE
   AND s.policy->>'createWhen' = 'jit';
