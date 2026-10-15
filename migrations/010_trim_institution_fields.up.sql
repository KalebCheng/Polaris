-- 010: 精简组织（institutions）只写不读的冗余字段。
-- 依据：docs/specs/261013-org-module-phase1.md §6 —— 这些字段仅入库/回显，运行时从不读取，
-- 属 AGENTS.md §1「配置即生效」红线要清理的对象（"配了但不生效"比"没有该配置"更危险）。
-- 说明：default_password 经核查在新建用户时被读取，故保留。
ALTER TABLE institutions
    DROP COLUMN languages,
    DROP COLUMN ip_whitelist,
    DROP COLUMN mfa_items,
    DROP COLUMN enable_soft_deletion,
    DROP COLUMN is_profile_public,
    DROP COLUMN master_password_hash,
    DROP COLUMN theme_data;
