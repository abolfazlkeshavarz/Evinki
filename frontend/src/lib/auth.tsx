import { createContext, useContext, useSyncExternalStore, type ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router-dom'
import { getSession, subscribe, type UserInfo } from './api'

export function useUser(): UserInfo | null {
  return useSyncExternalStore(subscribe, () => getSession()?.user ?? null)
}

export function useCan() {
  const u = useUser()
  return (...perms: string[]) => !!u && perms.some((p) => u.permissions.includes(p))
}

/** Where each kind of person lands after signing in. */
export function homeFor(u: UserInfo | null): string {
  if (!u) return '/login'
  const has = (p: string) => u.permissions.includes(p)
  if (u.roles.includes('technician') && !u.roles.some((r) => ['admin', 'super_admin', 'technical'].includes(r))) return '/tech'
  if (u.roles.includes('storekeeper') && u.roles.length === 1) return '/admin/warehouse/scan'
  if (has('reports.read') || has('shop.read') || has('service.read') || has('wh.read') || has('hr.read') || has('users.manage')) return '/admin'
  if (has('tech.portal')) return '/tech'
  return '/account'
}

export function RequireAuth({ perms, children }: { perms?: string[]; children: ReactNode }) {
  const u = useUser()
  const loc = useLocation()
  if (!u) return <Navigate to="/login" state={{ from: loc.pathname }} replace />
  if (perms && !perms.some((p) => u.permissions.includes(p))) return <Navigate to={homeFor(u)} replace />
  return <>{children}</>
}

const Ctx = createContext<null>(null)
export const AuthProvider = ({ children }: { children: ReactNode }) => <Ctx.Provider value={null}>{children}</Ctx.Provider>
