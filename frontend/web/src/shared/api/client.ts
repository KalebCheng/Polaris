import type { ApiResponse } from './types'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

/** 接口错误：携带后端业务码，便于前端按 code 走 i18n 分支 */
export class ApiError extends Error {
  readonly code: string
  readonly status: number
  readonly details?: Record<string, unknown>

  constructor(code: string, message: string, status: number, details?: Record<string, unknown>) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.details = details
  }
}

interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
  query?: Record<string, string | number | undefined | null>
}

function buildUrl(path: string, query?: RequestOptions['query']): string {
  const base = path.startsWith('http') ? path : `${API_BASE}${path}`
  if (!query) return base
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== '') params.set(key, String(value))
  }
  const qs = params.toString()
  return qs ? `${base}?${qs}` : base
}

/**
 * 统一请求封装：
 * - 会话走 HttpOnly Cookie（credentials: include），前端不接触 JWT；
 * - 成功响应解包 data；失败响应抛出 ApiError（带业务码）。
 */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { body, query, headers, ...rest } = options
  const response = await fetch(buildUrl(path, query), {
    ...rest,
    credentials: 'include',
    headers: {
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
      ...headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  return parseResponse<T>(response)
}

/** 解析统一响应：成功解包 data，失败抛出 ApiError（含业务码）。 */
async function parseResponse<T>(response: Response): Promise<T> {
  const text = await response.text()
  let payload: ApiResponse<T> | undefined
  if (text) {
    try {
      payload = JSON.parse(text) as ApiResponse<T>
    } catch {
      payload = undefined
    }
  }

  if (!response.ok) {
    throw new ApiError(
      payload?.code ?? 'INTERNAL_ERROR',
      payload?.message ?? `请求失败（HTTP ${response.status}）`,
      response.status,
      payload?.details,
    )
  }
  if (!payload) return undefined as T
  if (payload.code !== 'OK') {
    throw new ApiError(payload.code, payload.message, response.status, payload.details)
  }
  return payload.data as T
}

export const api = {
  get: <T>(path: string, query?: RequestOptions['query']) => request<T>(path, { method: 'GET', query }),
  post: <T>(path: string, body?: unknown, query?: RequestOptions['query']) =>
    request<T>(path, { method: 'POST', body, query }),
  put: <T>(path: string, body?: unknown, query?: RequestOptions['query']) =>
    request<T>(path, { method: 'PUT', body, query }),
  del: <T>(path: string, query?: RequestOptions['query']) => request<T>(path, { method: 'DELETE', query }),
  /**
   * 上传文件（multipart/form-data）：不走 JSON 序列化，直接提交 FormData；query 用于携带组织。
   * onUploadProgress 报告请求体上传进度（0–1）；响应解析完成后才 resolve。
   */
  upload: <T>(path: string, formData: FormData, opts?: RequestOptions['query'] | UploadOptions) => {
    const { query, onUploadProgress } = normalizeUploadOptions(opts)
    return uploadWithProgress<T>(buildUrl(path, query), formData, onUploadProgress)
  },
}

interface UploadOptions {
  query?: RequestOptions['query']
  /** 请求体上传进度（0–1） */
  onUploadProgress?: (ratio: number) => void
}

function normalizeUploadOptions(opts?: RequestOptions['query'] | UploadOptions): UploadOptions {
  if (!opts) return {}
  if ('query' in opts || 'onUploadProgress' in opts) return opts as UploadOptions
  return { query: opts as RequestOptions['query'] }
}

/**
 * XHR 上传：fetch 无法报告上传进度，而导入的文件可能数 MB、服务端解析还要十几秒，
 * 用户需要看到"正在上传 → 正在导入"两个阶段。
 */
function uploadWithProgress<T>(
  url: string,
  formData: FormData,
  onUploadProgress?: (ratio: number) => void,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', url)
    xhr.withCredentials = true
    if (onUploadProgress) {
      xhr.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable && event.total > 0) onUploadProgress(event.loaded / event.total)
      })
    }
    xhr.addEventListener('load', () => {
      const response = new Response(xhr.responseText, {
        status: xhr.status,
        headers: {
          'Content-Type': xhr.getResponseHeader('Content-Type') ?? 'application/json',
        },
      })
      parseResponse<T>(response).then(resolve, reject)
    })
    xhr.addEventListener('error', () => reject(new ApiError('NETWORK_ERROR', '网络错误，请稍后重试', 0)))
    xhr.send(formData)
  })
}

export { API_BASE }
