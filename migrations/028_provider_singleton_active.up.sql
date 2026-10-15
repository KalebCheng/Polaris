-- 028：单例类别在同一组织内最多一个"启用"项。
--
-- 背景（2026-10-13）：通知（邮件/短信）、人机校验、存储这四类在运行时都是"取该类别**首个启用项**"
-- （StorageService.activeStorageProvider / AuthService.notificationProvider / CaptchaService.ActiveProvider
--  → provider.Runtime.Active → 仓库 ORDER BY category, name 取第一个 active）。
-- 于是"配了多个启用的存储提供商，上传到底用哪个"完全取决于名称排序，界面上看不出来。
-- 外部身份提供方（oauth）相反：运行时是"列出全部启用项供登录页选择"，允许多个。
--
-- 处置：
--   1) 存量收敛：每个 (组织, 单例类别) 只保留**名称最小**的那个启用（与运行时现有的选择规则一致，
--      因此不会改变当前实际生效的提供商），其余置为 disabled；
--   2) 用部分唯一索引把规则落到数据库层（AGENTS §5：约束在迁移中声明，不依赖应用层兜底）。
--
-- 回滚：028_provider_singleton_active.down.sql（仅删除索引；被停用的行保持停用，避免"回滚即启用两个"）

-- ── 1. 存量收敛（幂等：重复执行不会改动已收敛的数据）─────────────────────────
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY institution_id, category ORDER BY name ASC, id ASC) AS rn
      FROM providers
     WHERE state = 'active'
       AND category IN ('email', 'sms', 'captcha', 'storage')
)
UPDATE providers p
   SET state = 'disabled', updated_at = NOW()
  FROM ranked r
 WHERE p.id = r.id AND r.rn > 1;

-- ── 2. 数据库层约束：单例类别 + 启用 → 每组织至多一行 ───────────────────────
CREATE UNIQUE INDEX IF NOT EXISTS providers_singleton_active_key
    ON providers (institution_id, category)
 WHERE state = 'active' AND category IN ('email', 'sms', 'captcha', 'storage');
