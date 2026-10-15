import type { AsyncdWorker } from '../api'

type BadgeVariant = 'success' | 'destructive' | 'secondary'

/**
 * 执行实例状态 → i18n 文案 key。
 * 与后端 asyncd_job_runs.status 一一对应：queued | running | succeeded | dead。
 */
export const ASYNCD_RUN_STATUS_KEYS: Record<string, string> = {
  queued: 'asyncd.statusQueued',
  running: 'asyncd.statusRunning',
  succeeded: 'asyncd.statusSucceeded',
  dead: 'asyncd.statusDead',
}

/** 任务卡片与筛选器共用的展示顺序 */
export const ASYNCD_RUN_STATUS_ORDER = ['queued', 'running', 'succeeded', 'dead'] as const

/** 状态徽标语气：成功=成功、死信=危险、执行中/等待中=中性 */
export function asyncdRunStatusVariant(status: string): BadgeVariant {
  if (status === 'succeeded') return 'success'
  if (status === 'dead') return 'destructive'
  return 'secondary'
}

/** 执行记录（单次尝试）状态 → 文案 key：running | succeeded | failed | lease_lost */
export const ASYNCD_ATTEMPT_STATUS_KEYS: Record<string, string> = {
  running: 'asyncd.attemptRunning',
  succeeded: 'asyncd.attemptSucceeded',
  failed: 'asyncd.attemptFailed',
  lease_lost: 'asyncd.attemptLeaseLost',
}

export function asyncdAttemptStatusVariant(status: string): BadgeVariant {
  if (status === 'succeeded') return 'success'
  if (status === 'failed') return 'destructive'
  if (status === 'lease_lost') return 'destructive'
  return 'secondary'
}

/** 执行器在线状态 → 文案 key：online | stale | stopped（由后端按心跳算出） */
export const ASYNCD_WORKER_STATE_KEYS: Record<string, string> = {
  online: 'asyncd.workerOnline',
  stale: 'asyncd.workerStale',
  stopped: 'asyncd.workerStopped',
}

export function asyncdWorkerStateVariant(state: AsyncdWorker['state'] | string): BadgeVariant {
  if (state === 'online') return 'success'
  if (state === 'stale') return 'destructive'
  return 'secondary'
}

/** 把毫秒耗时渲染成紧凑文本（数字 + 单位符号，无需翻译） */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '0ms'
  if (ms < 1000) return `${Math.round(ms)}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)}s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m${Math.round(seconds % 60)}s`
}
