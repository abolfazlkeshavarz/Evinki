import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, get } from './api'
import { useToast } from '../components/ui'

/** GET with caching. `path` is the cache key, so changing filters refetches. */
export function useGet<T = any>(path: string | null, opts?: { refetchInterval?: number }) {
  return useQuery<T>({ queryKey: ['api', path], queryFn: () => get<T>(path!), enabled: !!path, refetchInterval: opts?.refetchInterval })
}

/** Refresh every cached list/detail whose path starts with `prefix`. */
export function useInvalidate() {
  const qc = useQueryClient()
  return (prefix = '') => qc.invalidateQueries({ predicate: (q) => q.queryKey[0] === 'api' && String(q.queryKey[1] ?? '').startsWith(prefix) })
}

export function useSend<B = unknown, R = any>(method: string, path: string | ((b: B) => string), after?: (r: R, b: B) => void, invalidate = '') {
  const toast = useToast()
  const inv = useInvalidate()
  return useMutation<R, Error, B>({
    mutationFn: (b) => api<R>(method, typeof path === 'function' ? path(b) : path, b),
    onSuccess: (r, b) => { inv(invalidate); after?.(r, b) },
    onError: (e) => toast.error(e),
  })
}

export function qs(params: Record<string, string | number | undefined | null>): string {
  const u = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') u.set(k, String(v))
  const s = u.toString()
  return s ? '?' + s : ''
}
