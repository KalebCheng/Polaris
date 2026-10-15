-- 回滚：移除组织级密码盐列。
-- 注意：password_options 的枚举码归一化不可逆（原自由文本已丢失），此处不做还原。
ALTER TABLE institutions DROP COLUMN password_salt;
