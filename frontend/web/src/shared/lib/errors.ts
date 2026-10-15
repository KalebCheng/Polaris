import type { TFunction } from 'i18next'
import { ApiError } from '@/shared/api/client'

/**
 * 将后端业务码翻译为用户可读文案。
 * 约定：前端按 code 走 i18n，message 仅在无对应文案时兜底展示（后端 message 为英文）。
 */
export function describeError(error: unknown, t: TFunction): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case 'INVALID_CREDENTIALS':
        return t('auth.invalidCredentials')
      case 'TOO_MANY_REQUESTS':
        return t('auth.tooManyRequests')
      case 'USER_DISABLED':
        return t('auth.userDisabled')
      case 'PASSWORD_EXPIRED':
        return t('auth.passwordExpired')
      case 'INSTITUTION_REQUIRED':
        return t('auth.institutionAmbiguous')
      case 'MFA_INVALID':
        return t('auth.mfaInvalid')
      case 'MFA_REQUIRED':
        return t('auth.mfaTitle')
      case 'APPLICATION_NOT_FOUND':
        return t('auth.applicationMissing')
      case 'UNAUTHORIZED':
      case 'SESSION_EXPIRED':
      case 'SESSION_NOT_FOUND':
      case 'INVALID_TOKEN':
      case 'TOKEN_EXPIRED':
        return t('auth.unauthorized')
      case 'FORBIDDEN':
      case 'PERMISSION_DENIED':
      case 'PROTECTED_OBJECT':
        return t('auth.forbidden')
      default:
        return error.message || t('common.operationFailed')
    }
  }
  if (error instanceof Error) return error.message
  return t('common.operationFailed')
}
