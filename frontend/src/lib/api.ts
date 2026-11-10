// One API client for every screen (and a template for the future mobile apps):
// bearer token in memory + localStorage, one silent refresh on 401, uniform errors.

export interface UserInfo {
  id: number
  username: string
  full_name: string
  phone: string
  roles: string[]
  permissions: string[]
  must_change_password: boolean
}
interface Tokens {
  access_token: string
  refresh_token: string
  user: UserInfo
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public data?: any) {
    super(message)
  }
}

const KEY = 'evinki.session'
let session: Tokens | null = null
try {
  session = JSON.parse(localStorage.getItem(KEY) || 'null')
} catch {
  session = null
}
const listeners = new Set<() => void>()

export function getSession() {
  return session
}
export function setSession(t: Tokens | null) {
  session = t
  try {
    t ? localStorage.setItem(KEY, JSON.stringify(t)) : localStorage.removeItem(KEY)
  } catch {
    /* private mode: stay signed in for this tab only */
  }
  listeners.forEach((l) => l())
}
export function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

// Guests keep a cart through a random token.
export function cartToken(): string {
  let t = ''
  try {
    t = localStorage.getItem('evinki.cart') || ''
    if (t.length < 16) {
      t = crypto.randomUUID().replace(/-/g, '') + crypto.randomUUID().replace(/-/g, '')
      localStorage.setItem('evinki.cart', t)
    }
  } catch {
    t = 'volatile-' + Math.random().toString(36).slice(2) + Math.random().toString(36).slice(2)
  }
  return t
}

let refreshing: Promise<boolean> | null = null
async function refresh(): Promise<boolean> {
  if (!session) return false
  refreshing ??= (async () => {
    try {
      const r = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: session!.refresh_token }),
      })
      if (!r.ok) throw new Error()
      setSession(await r.json())
      return true
    } catch {
      setSession(null)
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

async function raw(method: string, path: string, body?: unknown, retry = true): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json', 'X-Cart-Token': cartToken() }
  if (body !== undefined && !(body instanceof FormData)) headers['Content-Type'] = 'application/json'
  if (session) headers.Authorization = 'Bearer ' + session.access_token
  const res = await fetch('/api/v1' + path, {
    method,
    headers,
    body: body === undefined ? undefined : body instanceof FormData ? body : JSON.stringify(body),
  })
  if (res.status === 401 && retry && session && (await refresh())) return raw(method, path, body, false)
  return res
}

export async function api<T = any>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response
  try {
    res = await raw(method, path, body)
  } catch {
    throw new ApiError(0, 'network', 'ارتباط با سرور برقرار نشد')
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  let data: any = undefined
  try {
    data = text ? JSON.parse(text) : undefined
  } catch {
    /* non-JSON */
  }
  if (!res.ok) {
    throw new ApiError(res.status, data?.error?.code ?? 'error', data?.error?.message ?? 'خطای ناشناخته', data)
  }
  return data as T
}

export const get = <T = any>(p: string) => api<T>('GET', p)
export const post = <T = any>(p: string, b?: unknown) => api<T>('POST', p, b ?? {})
export const put = <T = any>(p: string, b?: unknown) => api<T>('PUT', p, b ?? {})
export const patch = <T = any>(p: string, b?: unknown) => api<T>('PATCH', p, b ?? {})
export const del = <T = any>(p: string, b?: unknown) => api<T>('DELETE', p, b)

export async function upload(file: File, purpose = ''): Promise<{ id: string; url: string }> {
  const fd = new FormData()
  fd.append('file', file)
  const res = await raw('POST', '/files?purpose=' + purpose, fd)
  if (!res.ok) throw new ApiError(res.status, 'upload', (await res.json().catch(() => null))?.error?.message ?? 'بارگذاری ناموفق بود')
  return res.json()
}

// Login helpers ------------------------------------------------------------
export async function loginPassword(username: string, password: string) {
  const t = await api<Tokens>('POST', '/auth/login', { username, password, client: 'web' })
  setSession(t)
  return t.user
}
export const requestOtp = (phone: string) => api<{ expires_in: number; dev_code?: string }>('POST', '/auth/otp/request', { phone })
export async function verifyOtp(phone: string, code: string, name?: string) {
  const t = await api<Tokens>('POST', '/auth/otp/verify', { phone, code, name, client: 'web' })
  setSession(t)
  return t.user
}
export async function logout() {
  const s = session
  setSession(null)
  if (s) await fetch('/api/v1/auth/logout', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refresh_token: s.refresh_token }) }).catch(() => {})
}

/** Download an authenticated file (CSV/TXT exports) through a temporary link. */
export async function download(path: string, filename: string) {
  const res = await raw('GET', path)
  if (!res.ok) throw new ApiError(res.status, 'download', (await res.json().catch(() => null))?.error?.message ?? 'دریافت فایل ناموفق بود')
  const url = URL.createObjectURL(await res.blob())
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 5000)
}
