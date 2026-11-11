import { useState, type ReactNode } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { GlobalSearch } from './components/GlobalSearch'
import {
  Activity, BadgeCheck, BarChart3, Boxes, ClipboardList, Cog, FileText, Home, Layers, LogOut, Menu, Moon, Package, ScanLine, Search,
  ShieldCheck, ShoppingBag, ShoppingCart, Star, Sun, Tag, Truck, User, Users, Wallet, Warehouse, Wrench, X, Bell, Clock, Calendar, Fingerprint, Store, ListChecks, Settings2, History, PackageOpen, Trash2,
} from 'lucide-react'
import clsx from 'clsx'
import { homeFor, useCan, useUser } from './lib/auth'
import { logout } from './lib/api'
import { useGet } from './lib/hooks'
import { fa } from './lib/format'

export function Logo({ className }: { className?: string }) {
  return (
    <span className={clsx('inline-flex items-center gap-2 font-extrabold tracking-tight', className)}>
      <img src="/logo-evinki.png" alt="" className="h-9 w-9 rounded-full" width={36} height={36} />
      <span>اوینکی</span>
    </span>
  )
}

export function ThemeToggle() {
  const [dark, setDark] = useState(() => (document.documentElement.dataset.theme ?? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')) === 'dark')
  return (
    <button className="rounded-lg p-2 text-soft hover:bg-muted" aria-label="تغییر تم" onClick={() => {
      const next = dark ? 'light' : 'dark'
      document.documentElement.dataset.theme = next
      try { localStorage.setItem('evinki.theme', next) } catch { /* ignore */ }
      setDark(!dark)
    }}>{dark ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}</button>
  )
}

// ───────────────────────── customer account ─────────────────────────
export function AccountLayout() {
  const u = useUser()
  const nav = useNavigate()
  const items = [['/account', 'پیشخوان', Home], ['/account/orders', 'سفارش‌ها', Package], ['/account/guarantees', 'گارانتی‌ها', BadgeCheck], ['/account/wishlist', 'علاقه‌مندی‌ها', Star], ['/account/addresses', 'نشانی‌ها', Store], ['/account/profile', 'پروفایل', User]] as const
  return (
    <div className="grid gap-6 md:grid-cols-[220px_1fr]">
      <aside className="card h-fit p-2">
        <div className="mb-1 px-3 py-3"><div className="font-bold">{u?.full_name || 'مشتری'}</div><div className="text-xs text-soft num" dir="ltr">{u?.phone}</div></div>
        <nav className="flex gap-1 overflow-x-auto md:flex-col">
          {items.map(([to, l, Icon]) => (
            <NavLink key={to} to={to} end={to === '/account'} className={({ isActive }) => clsx('flex shrink-0 items-center gap-2 rounded-xl px-3 py-2.5 text-sm font-medium', isActive ? 'bg-brand-tint text-brand' : 'text-soft hover:bg-muted')}>
              <Icon className="h-4 w-4" />{l}
            </NavLink>
          ))}
          <button className="flex shrink-0 items-center gap-2 rounded-xl px-3 py-2.5 text-sm font-medium text-bad hover:bg-muted" onClick={async () => { await logout(); nav('/') }}><LogOut className="h-4 w-4" />خروج</button>
        </nav>
      </aside>
      <section><Outlet /></section>
    </div>
  )
}

// ───────────────────────── admin ─────────────────────────
type NavItem = { to: string; label: string; icon: any; perms: string[]; end?: boolean }
const sections: { title: string; items: NavItem[] }[] = [
  { title: '', items: [{ to: '/admin', label: 'نمای کلی', icon: BarChart3, perms: ['reports.read'], end: true }] },
  { title: 'فروشگاه', items: [
    { to: '/admin/orders', label: 'سفارش‌ها', icon: ShoppingBag, perms: ['shop.read', 'shop.manage'] },
    { to: '/admin/products', label: 'محصولات', icon: Layers, perms: ['catalog.manage'] },
    { to: '/admin/categories', label: 'دسته‌بندی‌ها', icon: Tag, perms: ['catalog.manage'] },
    { to: '/admin/coupons', label: 'کد تخفیف', icon: Tag, perms: ['shop.manage'] },
  ] },
  { title: 'گارانتی', items: [
    { to: '/admin/guarantees', label: 'گارانتی‌ها', icon: BadgeCheck, perms: ['warranty.read'] },
    { to: '/admin/customers', label: 'مشتریان', icon: Users, perms: ['warranty.read', 'service.read'] },
    { to: '/admin/trash', label: 'سطل بازیافت', icon: Trash2, perms: ['warranty.delete'] },
  ] },
  { title: 'خدمات پس از فروش', items: [
    { to: '/admin/service', label: 'پیگیری پرونده‌ها', icon: Wrench, perms: ['service.read'] },
    { to: '/admin/part-requests', label: 'درخواست قطعه', icon: PackageOpen, perms: ['service.read'] },
    { to: '/admin/shipments', label: 'مرسوله‌ها و پرداخت', icon: Truck, perms: ['service.read'] },
    { to: '/admin/technicians', label: 'تکنسین‌ها', icon: Users, perms: ['service.read'] },
  ] },
  { title: 'انبار', items: [
    { to: '/admin/warehouse', label: 'موجودی', icon: Warehouse, perms: ['wh.read'], end: true },
    { to: '/admin/warehouse/scan', label: 'ثبت حرکت / اسکن', icon: ScanLine, perms: ['wh.request', 'wh.write'] },
    { to: '/admin/warehouse/documents', label: 'اسناد انبار', icon: FileText, perms: ['wh.read'] },
    { to: '/admin/warehouse/counts', label: 'انبارگردانی', icon: ListChecks, perms: ['wh.read'] },
    { to: '/admin/parts', label: 'کاتالوگ قطعات', icon: Boxes, perms: ['catalog.manage', 'wh.manage'] },
    { to: '/admin/warehouse/labels', label: 'برچسب QR', icon: ScanLine, perms: ['wh.manage'] },
  ] },
  { title: 'منابع انسانی', items: [
    { to: '/admin/hr', label: 'داشبورد حضور', icon: Activity, perms: ['hr.read'], end: true },
    { to: '/admin/hr/attendance', label: 'ترددها', icon: Clock, perms: ['hr.read'] },
    { to: '/admin/hr/employees', label: 'پرسنل', icon: Users, perms: ['hr.read'] },
    { to: '/admin/hr/report', label: 'گزارش و نمودار', icon: Activity, perms: ['hr.read'] },
    { to: '/admin/hr/leaves', label: 'مرخصی‌ها', icon: Calendar, perms: ['hr.read'] },
    { to: '/admin/hr/tasks', label: 'وظایف', icon: ClipboardList, perms: ['hr.read'] },
    { to: '/admin/hr/devices', label: 'دستگاه‌های حضور', icon: Fingerprint, perms: ['hr.read'] },
    { to: '/admin/hr/shifts', label: 'شیفت و تعطیلات', icon: Clock, perms: ['hr.read'] },
  ] },
  { title: 'نظرسنجی', items: [{ to: '/admin/surveys', label: 'نظرسنجی‌ها', icon: Star, perms: ['surveys.read'] }] },
  { title: 'سامانه', items: [
    { to: '/admin/users', label: 'کاربران', icon: Users, perms: ['users.manage'] },
    { to: '/admin/roles', label: 'نقش‌ها و دسترسی', icon: ShieldCheck, perms: ['roles.manage'] },
    { to: '/admin/sms', label: 'الگوهای پیامک', icon: Bell, perms: ['settings.manage'] },
    { to: '/admin/settings', label: 'تنظیمات', icon: Settings2, perms: ['settings.manage'] },
    { to: '/admin/system-update', label: 'به‌روزرسانی سیستم', icon: PackageOpen, perms: ['settings.manage'] },
    { to: '/admin/audit', label: 'گزارش ممیزی', icon: History, perms: ['audit.read'] },
  ] },
]

export function AdminLayout() {
  const u = useUser()
  const can = useCan()
  const nav = useNavigate()
  const [open, setOpen] = useState(false)
  const notes = useGet<{ unread: number }>('/notifications', { refetchInterval: 60_000 })
  const menu = (
    <nav className="space-y-4 p-3" aria-label="منوی مدیریت">
      {sections.map((s) => {
        const items = s.items.filter((i) => can(...i.perms))
        if (!items.length) return null
        return (
          <div key={s.title}>
            {s.title && <div className="mb-1 px-3 text-[11px] font-bold uppercase tracking-wide text-soft">{s.title}</div>}
            {items.map((i) => (
              <NavLink key={i.to} to={i.to} end={i.end} onClick={() => setOpen(false)}
                className={({ isActive }) => clsx('flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium', isActive ? 'bg-brand-tint text-brand' : 'text-soft hover:bg-muted hover:text-ink')}>
                <i.icon className="h-4 w-4" />{i.label}
              </NavLink>
            ))}
          </div>
        )
      })}
    </nav>
  )
  return (
    <div className="min-h-screen md:grid md:grid-cols-[250px_1fr]">
      <aside className="sticky top-0 hidden h-screen overflow-y-auto border-l border-line bg-surface md:block">
        <div className="px-5 py-4"><Link to="/"><Logo className="text-lg" /></Link></div>
        {menu}
      </aside>
      {open && (
        <div className="fixed inset-0 z-40 bg-black/50 md:hidden" onClick={() => setOpen(false)}>
          <aside className="h-full w-72 overflow-y-auto bg-surface" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between px-5 py-4"><Logo /><button onClick={() => setOpen(false)} aria-label="بستن"><X className="h-5 w-5" /></button></div>
            {menu}
          </aside>
        </div>
      )}
      <div className="min-w-0">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-line bg-surface/90 px-4 backdrop-blur">
          <button className="rounded-lg p-2 md:hidden" onClick={() => setOpen(true)} aria-label="منو"><Menu className="h-5 w-5" /></button>
          <GlobalSearch />
          <div className="mr-auto flex items-center gap-1">
            <ThemeToggle />
            <span className="relative rounded-lg p-2 text-soft" aria-label="اعلان‌ها"><Bell className="h-5 w-5" />{!!notes.data?.unread && <span className="absolute right-1 top-1 h-2.5 w-2.5 rounded-full bg-bad" />}</span>
            <span className="hidden px-2 text-sm font-medium sm:block">{u?.full_name || u?.username}</span>
            <button className="rounded-lg p-2 text-soft hover:bg-muted" aria-label="خروج" onClick={async () => { await logout(); nav('/login') }}><LogOut className="h-5 w-5" /></button>
          </div>
        </header>
        <main className="p-4 md:p-6"><Outlet /></main>
      </div>
    </div>
  )
}

// ───────────────────────── technician (phone first) ─────────────────────────
export function TechLayout() {
  const u = useUser()
  const nav = useNavigate()
  const tabs = [['/tech', 'خانه', Home, true], ['/tech/jobs', 'تعمیرها', Wrench, false], ['/tech/stock', 'انبار من', Boxes, false], ['/tech/returns', 'ارسال قطعه', Truck, false], ['/tech/account', 'حساب', Wallet, false]] as const
  return (
    <div className="mx-auto flex min-h-screen max-w-2xl flex-col">
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-line bg-surface/95 px-4 backdrop-blur">
        <Logo />
        <div className="flex items-center gap-1">
          <span className="text-sm font-medium">{u?.full_name}</span>
          <ThemeToggle />
          <button className="rounded-lg p-2 text-soft" aria-label="خروج" onClick={async () => { await logout(); nav('/login') }}><LogOut className="h-5 w-5" /></button>
        </div>
      </header>
      <main className="flex-1 p-4 pb-28"><Outlet /></main>
      <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-line bg-surface pb-[env(safe-area-inset-bottom)]" aria-label="ناوبری">
        <div className="mx-auto grid max-w-2xl grid-cols-5">
          {tabs.map(([to, l, Icon, end]) => (
            <NavLink key={to} to={to} end={end} className={({ isActive }) => clsx('flex min-h-[58px] flex-col items-center justify-center gap-0.5 text-[11px] font-semibold', isActive ? 'text-brand' : 'text-soft')}>
              <Icon className="h-5 w-5" />{l}
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  )
}

export function Center({ children }: { children: ReactNode }) {
  return <div className="grid min-h-screen place-items-center p-4"><div className="w-full max-w-md">{children}</div></div>
}
