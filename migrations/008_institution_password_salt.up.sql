-- 008: 组织级密码盐（password_salt）与密码强度策略枚举码归一化。
-- 背景：组织可配置 password_type（密码类型）与 password_salt（盐/胡椒），
-- 需在运行时真正参与哈希与校验（见 internal/auth/cred 与 AGENTS.md「配置即生效」）。
-- 同时把历史自由文本的 password_options 归一化为枚举码（AtLeast6/AtLeast8/Aa123/SpecialChar/NoRepeat），
-- 与 Casdoor 对齐，避免模糊文本解析带来的歧义。

ALTER TABLE institutions ADD COLUMN password_salt TEXT NOT NULL DEFAULT '';

-- 归一化既有强度策略文本为枚举码；空数组保持为空。
UPDATE institutions SET password_options = COALESCE((
    SELECT array_agg(DISTINCT mapped)
    FROM (
        SELECT CASE
            WHEN opt ILIKE '%uppercase%' OR opt ILIKE '%lowercase%'
                 OR opt ILIKE '%digit%' OR opt ILIKE '%number%'
                 OR opt ILIKE '%Aa123%' THEN 'Aa123'
            WHEN opt ILIKE '%special%' THEN 'SpecialChar'
            WHEN opt ILIKE '%repeat%' THEN 'NoRepeat'
            WHEN opt ILIKE '%6%' THEN 'AtLeast6'
            WHEN opt ILIKE '%8%' OR opt ILIKE '%character%' THEN 'AtLeast8'
            ELSE opt
        END AS mapped
        FROM unnest(password_options) AS opt
        WHERE opt <> ''
    ) t
), '{}') WHERE password_options <> '{}';
