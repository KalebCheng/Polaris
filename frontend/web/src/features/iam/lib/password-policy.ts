/**
 * 组织密码策略取值（与后端 internal/auth/cred 及 auth 的强度策略枚举保持一致）。
 * 单一来源：组织新建/编辑与组织详情设置共用，避免两处漂移。
 */

/** 密码类型（顺序按推荐度；bcrypt 为平台默认） */
export const PASSWORD_TYPES = [
  'bcrypt',
  'argon2id',
  'pbkdf2-salt',
  'pbkdf2-django',
  'salt',
  'sha512-salt',
  'md5-salt',
  'plain',
] as const

/** 密码强度策略选项（枚举码 → i18n 文案 key） */
export const PASSWORD_OPTIONS: { value: string; labelKey: string }[] = [
  { value: 'AtLeast6', labelKey: 'institutions.pwdOptAtLeast6' },
  { value: 'AtLeast8', labelKey: 'institutions.pwdOptAtLeast8' },
  { value: 'Aa123', labelKey: 'institutions.pwdOptAa123' },
  { value: 'SpecialChar', labelKey: 'institutions.pwdOptSpecialChar' },
  { value: 'NoRepeat', labelKey: 'institutions.pwdOptNoRepeat' },
]
