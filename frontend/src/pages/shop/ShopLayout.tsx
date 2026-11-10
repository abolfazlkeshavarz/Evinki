import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { ChevronDown, Grid2x2, Heart, Home, AtSign, MessageCircle, Phone, Search, ShieldCheck, ShoppingCart, User, Wrench, X } from 'lucide-react'
import clsx from 'clsx'
import { Logo, ThemeToggle } from '../../layouts'
import { homeFor, useUser } from '../../lib/auth'
import { fa, toman } from '../../lib/format'
import { qs, useGet } from '../../lib/hooks'

/** Live suggestions under the search box, like marketplace sites. */
function SearchBox({ onDone, autoFocus }: { onDone?: () => void; autoFocus?: boolean }) {
  const nav = useNavigate()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const [dq, setDq] = useState('')
  const box = useRef<HTMLDivElement>(null)
  useEffect(() => { const t = setTimeout(() => setDq(q.trim()), 250); return () => clearTimeout(t) }, [q])
  useEffect(() => {
    const h = (e: MouseEvent) => !box.current?.contains(e.target as Node) && setOpen(false)
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])
  const res = useGet<any>(dq.length >= 2 ? '/shop/products' + qs({ q: dq, limit: 5 }) : null)
  const go = (to: string) => { setOpen(false); setQ(''); onDone?.(); nav(to) }
  return (
    <div ref={box} className="relative w-full">
      <form onSubmit={(e) => { e.preventDefault(); q.trim() && go('/products?q=' + encodeURIComponent(q.trim())) }}>
        <Search className="pointer-events-none absolute right-3.5 top-3.5 h-5 w-5 text-soft" />
        <input autoFocus={autoFocus} className="min-h-[46px] w-full rounded-xl border-0 bg-muted pr-11 pl-3 text-sm placeholder:text-soft focus:ring-2 focus:ring-brand/40" placeholder="جستجو در محصولات اوینکی…" value={q}
          onChange={(e) => { setQ(e.target.value); setOpen(true) }} onFocus={() => setOpen(true)} aria-label="جستجو" role="combobox" aria-expanded={open} />
      </form>
      {open && dq.length >= 2 && (
        <div className="absolute inset-x-0 top-full z-50 mt-2 overflow-hidden rounded-2xl border border-line bg-surface shadow-2xl">
          {res.isLoading ? <div className="p-4 text-sm text-soft">در حال جستجو…</div> : !res.data?.items?.length ? <div className="p-4 text-sm text-soft">محصولی با این عبارت پیدا نشد.</div> : (
            <>
              {res.data.items.map((p: any) => (
                <button key={p.id} onClick={() => go(`/products/${p.slug}`)} className="flex w-full items-center gap-3 border-b border-line/60 p-3 text-start hover:bg-muted">
                  <span className="grid h-11 w-11 shrink-0 place-items-center overflow-hidden rounded-lg bg-muted">{p.images?.[0] ? <img src={p.images[0]} alt="" className="h-full w-full object-cover" /> : <Search className="h-4 w-4 text-soft" />}</span>
                  <span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold">{p.name}</span><span className="text-xs text-soft">{p.category_name}</span></span>
                  <span className="text-xs font-bold num">{p.price_on_request || !p.price ? 'تماس بگیرید' : toman(p.price)}</span>
                </button>
              ))}
              <button onClick={() => go('/products?q=' + encodeURIComponent(dq))} className="w-full p-3 text-sm font-semibold text-brand hover:bg-muted">مشاهده همه نتایج «{dq}»</button>
            </>
          )}
        </div>
      )}
    </div>
  )
}

function MiniCart() {
  const cart = useGet<any>('/shop/cart')
  const [open, setOpen] = useState(false)
  const loc = useLocation()
  useEffect(() => setOpen(false), [loc.pathname])
  const c = cart.data
  return (
    <div className="relative" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <Link to="/cart" className="relative flex rounded-xl p-2.5 text-ink hover:bg-muted" aria-label="سبد خرید">
        <ShoppingCart className="h-6 w-6" />
        {!!c?.count && <span className="absolute -left-0.5 -top-0.5 grid h-5 min-w-5 place-items-center rounded-full bg-brand px-1 text-[11px] font-bold text-brand-ink num">{fa(c.count)}</span>}
      </Link>
      {open && (
        <div className="absolute left-0 top-full z-50 hidden w-80 pt-2 md:block">
          <div className="rounded-2xl border border-line bg-surface p-3 shadow-2xl">
            {!c?.lines?.length ? <p className="p-4 text-center text-sm text-soft">سبد خرید شما خالی است</p> : (
              <>
                <div className="max-h-64 divide-y divide-line overflow-y-auto">
                  {c.lines.slice(0, 5).map((l: any) => (
                    <div key={l.id} className="flex items-center gap-3 py-2.5">
                      <span className="h-12 w-12 shrink-0 overflow-hidden rounded-lg bg-muted">{l.image && <img src={l.image} alt="" className="h-full w-full object-cover" />}</span>
                      <span className="min-w-0 flex-1 text-xs"><span className="line-clamp-2 font-semibold">{l.name}</span><span className="text-soft num">{fa(l.qty)} × {toman(l.unit_price)}</span></span>
                    </div>
                  ))}
                </div>
                <div className="mt-2 flex items-center justify-between border-t border-line pt-3 text-sm"><span className="text-soft">جمع</span><b className="num">{toman(c.subtotal)}</b></div>
                <Link to="/cart" className="btn-primary mt-3 w-full">مشاهده سبد و پرداخت</Link>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export function ShopLayout() {
  const u = useUser()
  const cats = useGet<any[]>('/shop/categories')
  const cart = useGet<any>('/shop/cart')
  const [catOpen, setCatOpen] = useState(false)
  const [mSearch, setMSearch] = useState(false)
  const loc = useLocation()
  useEffect(() => { setCatOpen(false); setMSearch(false); window.scrollTo(0, 0) }, [loc.pathname])
  const nav = ({ isActive }: { isActive: boolean }) => clsx('rounded-lg px-3 py-2 text-sm font-semibold transition', isActive ? 'text-brand' : 'text-ink/80 hover:text-brand')
  const tab = ({ isActive }: { isActive: boolean }) => clsx('relative flex min-h-[56px] flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-semibold', isActive ? 'text-brand' : 'text-soft')
  return (
    <div className="flex min-h-screen flex-col pb-[calc(58px+env(safe-area-inset-bottom))] md:pb-0">
      <div className="hidden bg-brand-dark text-white md:block"><div className="mx-auto flex h-9 max-w-7xl items-center justify-between px-4 text-xs">
        <span className="flex items-center gap-1.5"><ShieldCheck className="h-3.5 w-3.5" />گارانتی ۱۸ ماهه + ۳ ماه گارانتی طلایی روی همه جاروبرقی‌ها</span>
        <span className="flex items-center gap-4"><a href="tel:02833362824" className="flex items-center gap-1 num"><Phone className="h-3.5 w-3.5" />۰۲۸-۳۳۳۶۲۸۲۴</a><Link to="/technician/apply" className="hover:underline">همکاری تکنسین</Link></span>
      </div></div>

      <header className="sticky top-0 z-40 border-b border-line bg-surface">
        <div className="mx-auto flex h-16 max-w-7xl items-center gap-3 px-4">
          <Link to="/" aria-label="اوینکی"><Logo className="text-xl" /></Link>
          <div className="mx-4 hidden max-w-2xl flex-1 md:block"><SearchBox /></div>
          <div className="mr-auto flex items-center gap-0.5">
            <button className="rounded-xl p-2.5 md:hidden" aria-label="جستجو" onClick={() => setMSearch((v) => !v)}>{mSearch ? <X className="h-6 w-6" /> : <Search className="h-6 w-6" />}</button>
            <span className="hidden md:block"><ThemeToggle /></span>
            {u && <Link to="/account/wishlist" className="hidden rounded-xl p-2.5 hover:bg-muted md:flex" aria-label="علاقه‌مندی‌ها"><Heart className="h-6 w-6" /></Link>}
            <MiniCart />
            <span className="mx-1 hidden h-7 w-px bg-line md:block" />
            {u ? <Link to={homeFor(u)} className="btn-ghost btn-sm hidden md:inline-flex"><User className="h-4 w-4" />{u.full_name?.split(' ')[0] || 'حساب من'}</Link>
              : <Link to="/login" className="btn-primary btn-sm hidden md:inline-flex">ورود | ثبت‌نام</Link>}
          </div>
        </div>
        {mSearch && <div className="border-t border-line p-3 md:hidden"><SearchBox autoFocus onDone={() => setMSearch(false)} /></div>}

        <div className="relative hidden border-t border-line md:block"><div className="mx-auto flex h-11 max-w-7xl items-center gap-1 px-4">
          <button className="flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-bold hover:bg-muted" onClick={() => setCatOpen((v) => !v)} onMouseEnter={() => setCatOpen(true)}><Grid2x2 className="h-4 w-4" />دسته‌بندی کالاها<ChevronDown className="h-3.5 w-3.5" /></button>
          <span className="mx-1 h-5 w-px bg-line" />
          <NavLink to="/products" end className={nav}>همه محصولات</NavLink>
          <NavLink to="/accessories" className={nav}>لوازم جانبی و یدکی</NavLink>
          <NavLink to="/guarantee" className={nav}>گارانتی و استعلام</NavLink>
          <a className="rounded-lg px-3 py-2 text-sm font-semibold text-ink/80 hover:text-brand" href="https://evinki.com/" target="_blank" rel="noopener">وب‌سایت اوینکی</a>
        </div>
          {catOpen && (
            <div className="absolute inset-x-0 top-full z-40" onMouseLeave={() => setCatOpen(false)}>
              <div className="mx-auto max-w-7xl px-4"><div className="grid gap-1 rounded-b-2xl border border-t-0 border-line bg-surface p-4 shadow-2xl sm:grid-cols-2 lg:grid-cols-4">
                {cats.data?.map((c) => <Link key={c.id} to={`/products?category=${c.slug}`} className="rounded-xl p-3 hover:bg-muted"><div className="font-bold">{c.name}</div><div className="text-xs text-soft">{fa(c.product_count)} محصول</div></Link>)}
                <Link to="/accessories" className="rounded-xl p-3 hover:bg-muted"><div className="font-bold">لوازم جانبی و یدکی</div><div className="text-xs text-soft">فیلتر، خرطومی، چرخ و …</div></Link>
              </div></div>
            </div>
          )}
        </div>
      </header>

      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-5"><Outlet /></main>

      <section className="border-t border-line bg-surface"><div className="mx-auto grid max-w-7xl grid-cols-2 gap-4 px-4 py-6 md:grid-cols-4">
        {[[ShieldCheck, 'گارانتی اصل', '۱۸ ماه + ۳ ماه طلایی'], [Wrench, 'خدمات پس از فروش', 'نمایندگی در سراسر ایران'], [Home, 'تولید ایران', 'استاندارد اروپایی'], [Phone, 'پشتیبانی', 'شنبه تا چهارشنبه ۹:۳۰ تا ۱۷:۳۰']].map(([I, t, d]: any) => (
          <div key={t} className="flex items-center gap-3"><div className="rounded-xl bg-brand-tint p-2.5 text-brand"><I className="h-6 w-6" /></div><div><div className="text-sm font-bold">{t}</div><div className="text-xs text-soft">{d}</div></div></div>
        ))}
      </div></section>
      <footer className="bg-ink text-bg/80"><div className="mx-auto grid max-w-7xl gap-8 px-4 py-10 text-sm md:grid-cols-4">
        <div className="md:col-span-2"><div className="mb-3 flex items-center gap-2 text-lg font-extrabold text-bg"><img src="/logo-evinki.png" alt="" className="h-10 w-10 rounded-full" />اوینکی</div>
          <p className="max-w-md leading-7">تولیدکننده جاروبرقی و لوازم خانگی با استاندارد اروپایی. خرید آنلاین، ثبت و استعلام گارانتی و درخواست خدمات پس از فروش، همه در یک‌جا.</p>
          <div className="mt-4 flex gap-3"><a href="https://www.instagram.com/evinkico/" target="_blank" rel="noopener" aria-label="اینستاگرام" className="rounded-full bg-white/10 p-2.5 hover:bg-brand"><AtSign className="h-5 w-5" /></a><a href="https://wa.me/989055365565" target="_blank" rel="noopener" aria-label="واتس‌اپ" className="rounded-full bg-white/10 p-2.5 hover:bg-brand"><MessageCircle className="h-5 w-5" /></a></div></div>
        <div className="space-y-2"><div className="font-bold text-bg">خرید</div><Link className="block hover:text-white" to="/products">همه محصولات</Link><Link className="block hover:text-white" to="/accessories">لوازم جانبی</Link><Link className="block hover:text-white" to="/cart">سبد خرید</Link><Link className="block hover:text-white" to="/account/orders">پیگیری سفارش</Link></div>
        <div className="space-y-2"><div className="font-bold text-bg">پشتیبانی</div><Link className="block hover:text-white" to="/guarantee">استعلام و ثبت گارانتی</Link><Link className="block hover:text-white" to="/technician/apply">همکاری تکنسین</Link><p className="num" dir="ltr">028-33362824</p><p>قزوین، خیابان امام، دفتر مرکزی</p><p>info@evinki.com</p></div>
      </div><div className="border-t border-white/10 py-4 text-center text-xs">© {new Date().getFullYear() - 621} اوینکی — تمامی حقوق محفوظ است</div></footer>

      {/* phones: marketplace-style bottom bar */}
      <nav className="fixed inset-x-0 bottom-0 z-40 flex border-t border-line bg-surface pb-[env(safe-area-inset-bottom)] md:hidden" aria-label="ناوبری">
        <NavLink to="/" end className={tab}><Home className="h-5 w-5" />خانه</NavLink>
        <NavLink to="/products" className={tab}><Grid2x2 className="h-5 w-5" />دسته‌بندی</NavLink>
        <NavLink to="/cart" className={tab}><span className="relative"><ShoppingCart className="h-5 w-5" />{!!cart.data?.count && <span className="absolute -right-2.5 -top-2 grid h-4 min-w-4 place-items-center rounded-full bg-brand px-1 text-[10px] text-brand-ink num">{fa(cart.data.count)}</span>}</span>سبد خرید</NavLink>
        <NavLink to={u ? homeFor(u) : '/login'} className={tab}><User className="h-5 w-5" />{u ? 'حساب من' : 'ورود'}</NavLink>
      </nav>
    </div>
  )
}
