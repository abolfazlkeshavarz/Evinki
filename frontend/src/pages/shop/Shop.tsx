import { useEffect, useState, type ReactNode } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { BadgeCheck, Check, ChevronLeft, ChevronRight, Heart, Minus, Package, Phone, Plus, Share2, ShieldCheck, SlidersHorizontal, Truck, Wrench, X } from 'lucide-react'
import clsx from 'clsx'
import { Badge, Card, Empty, ErrorBox, Loading, Pager, Tabs, useToast } from '../../components/ui'
import { del, put } from '../../lib/api'
import { useUser } from '../../lib/auth'
import { fa, toman } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'

export function useAddToCart() {
  const toast = useToast()
  const inv = useInvalidate()
  return async (item: { product_id?: number; part_id?: number }, qty = 1) => {
    try {
      await put('/shop/cart/items', { ...item, qty })
      inv('/shop/cart')
      toast.ok('به سبد خرید اضافه شد')
    } catch (e) {
      toast.error(e)
    }
  }
}

export function Price({ price, onRequest, compare, big }: { price: number; onRequest?: boolean; compare?: number; big?: boolean }) {
  if (onRequest || !price) return <span className={clsx('font-semibold text-soft', big ? 'text-lg' : 'text-sm')}>برای قیمت تماس بگیرید</span>
  const off = compare && compare > price ? Math.round(((compare - price) * 100) / compare) : 0
  return (
    <span className="flex flex-col items-end gap-0.5">
      {off > 0 && <span className="flex items-center gap-1.5"><s className="text-xs text-soft num">{fa(compare)}</s><span className="rounded-full bg-brand px-1.5 py-0.5 text-[11px] font-bold text-brand-ink num">{fa(off)}٪</span></span>}
      <span className={clsx('font-extrabold num', big ? 'text-2xl' : 'text-base')}>{fa(price)} <span className="text-xs font-semibold text-soft">تومان</span></span>
    </span>
  )
}

function Thumb({ src, name, className }: { src?: string; name: string; className?: string }) {
  return src ? <img src={src} alt={name} loading="lazy" className={clsx('h-full w-full object-cover', className)} />
    : <div className="grid h-full w-full place-items-center bg-gradient-to-br from-brand-tint to-muted"><img src="/logo-evinki.png" alt="" className="h-1/3 opacity-40 grayscale" /></div>
}

export function Crumbs({ items }: { items: { to?: string; label: string }[] }) {
  return (
    <nav aria-label="مسیر" className="mb-4 flex flex-wrap items-center gap-1.5 text-xs text-soft">
      <Link to="/" className="hover:text-brand">خانه</Link>
      {items.map((i, k) => <span key={k} className="flex items-center gap-1.5"><ChevronLeft className="h-3.5 w-3.5" />{i.to ? <Link to={i.to} className="hover:text-brand">{i.label}</Link> : <span className="text-ink">{i.label}</span>}</span>)}
    </nav>
  )
}

export function WishHeart({ id, className }: { id: number; className?: string }) {
  const u = useUser()
  const nav = useNavigate()
  const toast = useToast()
  const inv = useInvalidate()
  const wl = useGet<any[]>(u ? '/shop/wishlist' : null)
  const on = !!wl.data?.some((p) => p.id === id)
  return (
    <button aria-label={on ? 'حذف از علاقه‌مندی‌ها' : 'افزودن به علاقه‌مندی‌ها'} aria-pressed={on} className={clsx('grid h-9 w-9 place-items-center rounded-full bg-surface/90 shadow transition hover:scale-110', className)}
      onClick={async (e) => {
        e.preventDefault(); e.stopPropagation()
        if (!u) return nav('/login')
        try { on ? await del(`/shop/wishlist/${id}`) : await put(`/shop/wishlist/${id}`); inv('/shop/wishlist'); toast.ok(on ? 'از علاقه‌مندی‌ها حذف شد' : 'به علاقه‌مندی‌ها اضافه شد') } catch (x) { toast.error(x) }
      }}>
      <Heart className={clsx('h-5 w-5', on ? 'fill-brand text-brand' : 'text-soft')} />
    </button>
  )
}

export function ProductCard({ p, compact }: { p: any; compact?: boolean }) {
  const add = useAddToCart()
  const orderable = !p.price_on_request && p.price > 0
  return (
    <Card className={clsx('group relative flex flex-col overflow-hidden transition hover:shadow-lg', compact && 'w-44 shrink-0 snap-start sm:w-56')}>
      <WishHeart id={p.id} className="absolute left-2 top-2 z-10" />
      {p.compare_price > p.price && p.price > 0 && <span className="absolute right-2 top-2 z-10 rounded-full bg-brand px-2 py-0.5 text-xs font-bold text-brand-ink num">{fa(Math.round(((p.compare_price - p.price) * 100) / p.compare_price))}٪ تخفیف</span>}
      <Link to={`/products/${p.slug}`} className="block aspect-square overflow-hidden bg-muted"><Thumb src={p.images?.[0]} name={p.name} className="transition duration-300 group-hover:scale-105" /></Link>
      <div className="flex flex-1 flex-col gap-2 p-3">
        <Link to={`/products/${p.slug}`} className="line-clamp-2 min-h-[2.6rem] text-sm font-semibold leading-snug group-hover:text-brand">{p.name}</Link>
        <div className="flex items-center gap-1 text-[11px] text-ok"><ShieldCheck className="h-3.5 w-3.5" />گارانتی {fa(p.guarantee_months ?? 18)} ماهه</div>
        <div className="mt-auto flex items-end justify-between gap-2 pt-1">
          {orderable && p.in_stock ? <button className="btn-soft btn-sm !min-h-[34px] !px-2.5" aria-label="افزودن به سبد" onClick={() => add({ product_id: p.id })}><Plus className="h-4 w-4" /></button> : <span />}
          {!p.in_stock && orderable ? <span className="text-xs font-bold text-soft">ناموجود</span> : <Price price={p.price} onRequest={p.price_on_request} compare={p.compare_price} />}
        </div>
      </div>
    </Card>
  )
}

function Row({ title, to, children }: { title: string; to?: string; children: ReactNode }) {
  return (
    <section>
      <div className="mb-3 flex items-end justify-between"><h2 className="text-lg font-extrabold md:text-xl">{title}</h2>{to && <Link to={to} className="flex items-center gap-1 text-sm font-semibold text-brand">مشاهده همه<ChevronLeft className="h-4 w-4" /></Link>}</div>
      <div className="-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-2 [scrollbar-width:none]">{children}</div>
    </section>
  )
}

const slides = [
  { t: 'قدرتی که خانه را متحول می‌کند', s: 'جاروبرقی FZ530 Performance — موتور ۲۴۰۰ وات، فیلتر HEPA13 قابل شستشو', cta: 'مشاهده محصول', to: '/products/fz530-performance', bg: 'from-brand to-[#9f1118]' },
  { t: 'گارانتی ۱۸ ماهه + ۳ ماه طلایی', s: 'کد گارانتی را ثبت کنید و پشتیبانی سراسری را فعال کنید', cta: 'ثبت گارانتی', to: '/guarantee', bg: 'from-[#2a2a2a] to-[#111]' },
  { t: 'لوازم جانبی اوینکی', s: 'فیلتر هپا، خرطومی، چرخ و ملزومات مصرفی اصل', cta: 'خرید لوازم جانبی', to: '/accessories', bg: 'from-[#c4161f] to-[#ed242e]' },
]

function Hero() {
  const [i, setI] = useState(0)
  useEffect(() => { const t = setInterval(() => setI((v) => (v + 1) % slides.length), 6000); return () => clearInterval(t) }, [])
  const s = slides[i]
  return (
    <section className={clsx('relative overflow-hidden rounded-3xl bg-gradient-to-l p-7 text-white md:p-14', s.bg)} aria-roledescription="carousel">
      <img src="/logo-evinki.png" alt="" aria-hidden className="pointer-events-none absolute -left-10 -top-10 h-72 w-72 rounded-full opacity-15 md:h-96 md:w-96" />
      <div className="relative max-w-xl">
        <p className="mb-2 text-xs font-semibold opacity-90 md:text-sm">اولین تولیدکننده لوازم خانگی با استاندارد اروپایی در ایران</p>
        <h1 className="text-2xl font-extrabold leading-tight md:text-5xl">{s.t}</h1>
        <p className="mt-3 text-sm opacity-90 md:text-base">{s.s}</p>
        <Link to={s.to} className="btn mt-6 bg-white text-brand hover:bg-white/90">{s.cta}</Link>
      </div>
      <div className="absolute bottom-4 left-4 flex items-center gap-2">
        <button aria-label="قبلی" className="grid h-8 w-8 place-items-center rounded-full bg-white/20 hover:bg-white/30" onClick={() => setI((i + slides.length - 1) % slides.length)}><ChevronRight className="h-4 w-4" /></button>
        {slides.map((_, k) => <button key={k} aria-label={`اسلاید ${k + 1}`} onClick={() => setI(k)} className={clsx('h-2 rounded-full transition-all', k === i ? 'w-6 bg-white' : 'w-2 bg-white/50')} />)}
        <button aria-label="بعدی" className="grid h-8 w-8 place-items-center rounded-full bg-white/20 hover:bg-white/30" onClick={() => setI((i + 1) % slides.length)}><ChevronLeft className="h-4 w-4" /></button>
      </div>
    </section>
  )
}

export function Home() {
  const nav = useNavigate()
  const [code, setCode] = useState('')
  const cats = useGet<any[]>('/shop/categories')
  const news = useGet<any>('/shop/products?limit=10')
  const deals = useGet<any>('/shop/products?limit=10&priced=1&sort=price_asc')
  const parts = useGet<any[]>('/shop/parts?limit=10')
  return (
    <div className="space-y-10">
      <Hero />
      {!!cats.data?.length && (
        <section><div className="grid grid-cols-4 gap-3 md:grid-cols-8">
          {[...cats.data.map((c) => ({ k: c.id, to: `/products?category=${c.slug}`, l: c.name })), { k: 'acc', to: '/accessories', l: 'لوازم جانبی' }, { k: 'g', to: '/guarantee', l: 'استعلام گارانتی' }, { k: 'tech', to: '/technician/apply', l: 'همکاری تکنسین' }].map((c) => (
            <Link key={c.k} to={c.to} className="group flex flex-col items-center gap-2 text-center">
              <span className="grid h-16 w-16 place-items-center rounded-full bg-brand-tint text-brand transition group-hover:scale-105 group-hover:bg-brand group-hover:text-brand-ink md:h-20 md:w-20">{c.k === 'g' ? <BadgeCheck className="h-8 w-8" /> : c.k === 'tech' ? <Wrench className="h-8 w-8" /> : <Package className="h-8 w-8" />}</span>
              <span className="text-xs font-semibold md:text-sm">{c.l}</span>
            </Link>
          ))}
        </div></section>
      )}
      {news.isLoading ? <Loading /> : !!news.data?.items?.length ? (
        <Row title="جدیدترین محصولات" to="/products">{news.data.items.map((p: any) => <ProductCard key={p.id} p={p} compact />)}</Row>
      ) : <Empty title="هنوز محصولی منتشر نشده است" />}
      {!!deals.data?.items?.length && <Row title="محصولات قابل خرید آنلاین" to="/products?priced=1">{deals.data.items.map((p: any) => <ProductCard key={p.id} p={p} compact />)}</Row>}
      {!!parts.data?.length && (
        <Row title="لوازم جانبی و یدکی" to="/accessories">{parts.data.map((p) => (
          <Card key={p.id} className="w-44 shrink-0 snap-start overflow-hidden sm:w-56"><div className="aspect-square bg-muted"><Thumb src={p.image} name={p.name} /></div><div className="p-3"><div className="line-clamp-2 min-h-[2.4rem] text-sm font-semibold">{p.name}</div><div className="mt-2 text-end text-base font-extrabold num">{fa(p.price)} <span className="text-xs text-soft">تومان</span></div></div></Card>
        ))}</Row>
      )}
      <section className="overflow-hidden rounded-3xl bg-ink p-6 text-bg md:flex md:items-center md:justify-between md:p-10">
        <div><h2 className="flex items-center gap-2 text-xl font-extrabold"><ShieldCheck className="h-6 w-6 text-brand" />گارانتی دستگاه خود را بررسی کنید</h2><p className="mt-1 text-sm opacity-80">کد گارانتی روی دستگاه یا کارت گارانتی درج شده است.</p></div>
        <form className="mt-4 flex gap-2 md:mt-0 md:w-96" onSubmit={(e) => { e.preventDefault(); nav('/guarantee?code=' + encodeURIComponent(code)) }}>
          <input className="input !bg-white/10 !text-white placeholder:!text-white/50" dir="ltr" placeholder="کد گارانتی" value={code} onChange={(e) => setCode(e.target.value)} aria-label="کد گارانتی" /><button className="btn-primary">استعلام</button>
        </form>
      </section>
    </div>
  )
}

export function Products() {
  const [sp, setSp] = useSearchParams()
  const cats = useGet<any[]>('/shop/categories')
  const [drawer, setDrawer] = useState(false)
  const page = +(sp.get('page') || 1)
  const [price, setPrice] = useState({ min: sp.get('min') ?? '', max: sp.get('max') ?? '' })
  const q = useGet<any>('/shop/products' + qs({ q: sp.get('q'), category: sp.get('category'), sort: sp.get('sort'), min: sp.get('min'), max: sp.get('max'), stock: sp.get('stock'), priced: sp.get('priced'), page, limit: 12 }))
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); v ? n.set(k, v) : n.delete(k); n.delete('page'); setSp(n) }
  const cat = cats.data?.find((c) => c.slug === sp.get('category'))
  const chips = [sp.get('q') && { k: 'q', l: `«${sp.get('q')}»` }, cat && { k: 'category', l: cat.name }, sp.get('stock') && { k: 'stock', l: 'فقط موجود' }, sp.get('priced') && { k: 'priced', l: 'قابل خرید آنلاین' }, (sp.get('min') || sp.get('max')) && { k: 'price', l: 'محدوده قیمت' }].filter(Boolean) as { k: string; l: string }[]
  const filters = (
    <div className="space-y-6">
      <div><div className="mb-2 text-sm font-bold">دسته‌بندی</div>
        <div className="space-y-0.5">
          <button className={clsx('block w-full rounded-lg px-3 py-2 text-start text-sm', !sp.get('category') ? 'bg-brand-tint font-bold text-brand' : 'hover:bg-muted')} onClick={() => set('category', '')}>همه محصولات</button>
          {cats.data?.map((c) => <button key={c.id} className={clsx('flex w-full justify-between rounded-lg px-3 py-2 text-start text-sm', sp.get('category') === c.slug ? 'bg-brand-tint font-bold text-brand' : 'hover:bg-muted')} onClick={() => set('category', c.slug)}>{c.name}<span className="text-xs text-soft num">{fa(c.product_count)}</span></button>)}
        </div></div>
      <div><div className="mb-2 text-sm font-bold">محدوده قیمت (تومان)</div>
        <div className="flex gap-2"><input className="input num" dir="ltr" inputMode="numeric" placeholder="از" value={price.min} onChange={(e) => setPrice({ ...price, min: e.target.value.replace(/\D/g, '') })} /><input className="input num" dir="ltr" inputMode="numeric" placeholder="تا" value={price.max} onChange={(e) => setPrice({ ...price, max: e.target.value.replace(/\D/g, '') })} /></div>
        <button className="btn-ghost btn-sm mt-2 w-full" onClick={() => { const n = new URLSearchParams(sp); price.min ? n.set('min', price.min) : n.delete('min'); price.max ? n.set('max', price.max) : n.delete('max'); n.delete('page'); setSp(n); setDrawer(false) }}>اعمال</button></div>
      {([['stock', 'فقط کالاهای موجود'], ['priced', 'فقط قابل خرید آنلاین']] as const).map(([k, l]) => (
        <label key={k} className="flex cursor-pointer items-center justify-between text-sm font-medium">{l}
          <button role="switch" aria-checked={sp.get(k) === '1'} onClick={() => set(k, sp.get(k) === '1' ? '' : '1')} className={clsx('relative h-6 w-11 rounded-full transition', sp.get(k) === '1' ? 'bg-brand' : 'bg-line')}><span className={clsx('absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all', sp.get(k) === '1' ? 'right-[22px]' : 'right-0.5')} /></button></label>
      ))}
    </div>
  )
  return (
    <div>
      <Crumbs items={[{ label: cat ? cat.name : 'همه محصولات' }]} />
      <div className="grid gap-6 md:grid-cols-[250px_1fr]">
        <aside className="card hidden h-fit p-4 md:block">{filters}</aside>
        <section>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <button className="btn-ghost btn-sm md:hidden" onClick={() => setDrawer(true)}><SlidersHorizontal className="h-4 w-4" />فیلترها</button>
            <span className="text-sm font-bold text-soft">مرتب‌سازی:</span>
            <Tabs value={sp.get('sort') ?? ''} onChange={(v) => set('sort', v)} items={[{ key: '', label: 'جدیدترین' }, { key: 'price_asc', label: 'ارزان‌ترین' }, { key: 'price_desc', label: 'گران‌ترین' }]} />
            {q.data && <span className="mr-auto text-xs text-soft">{fa(q.data.total)} کالا</span>}
          </div>
          {!!chips.length && <div className="mb-3 flex flex-wrap gap-2">{chips.map((c) => <button key={c.k} className="flex items-center gap-1 rounded-full bg-brand-tint px-3 py-1 text-xs font-semibold text-brand" onClick={() => { if (c.k === 'price') { const n = new URLSearchParams(sp); n.delete('min'); n.delete('max'); setPrice({ min: '', max: '' }); setSp(n) } else set(c.k, '') }}>{c.l}<X className="h-3 w-3" /></button>)}<button className="text-xs text-soft underline" onClick={() => { setSp(new URLSearchParams()); setPrice({ min: '', max: '' }) }}>حذف همه</button></div>}
          {q.isLoading ? <Loading /> : q.error ? <ErrorBox error={q.error} /> : !q.data.items.length ? <Empty title="کالایی با این مشخصات پیدا نشد" hint="فیلترها را تغییر دهید یا جستجوی دیگری امتحان کنید." /> : (
            <>
              <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-4">{q.data.items.map((p: any) => <ProductCard key={p.id} p={p} />)}</div>
              <Card className="mt-4"><Pager page={page} total={q.data.total} limit={q.data.limit} onPage={(p) => { const n = new URLSearchParams(sp); n.set('page', String(p)); setSp(n); scrollTo(0, 0) }} /></Card>
            </>
          )}
        </section>
      </div>
      {drawer && (
        <div className="fixed inset-0 z-50 bg-black/50 md:hidden" onClick={() => setDrawer(false)}>
          <div className="absolute inset-x-0 bottom-0 max-h-[85vh] overflow-y-auto rounded-t-3xl bg-surface p-5" onClick={(e) => e.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between"><b>فیلترها</b><button onClick={() => setDrawer(false)} aria-label="بستن"><X className="h-5 w-5" /></button></div>{filters}
            <button className="btn-primary mt-5 w-full" onClick={() => setDrawer(false)}>نمایش {q.data ? fa(q.data.total) : ''} کالا</button>
          </div>
        </div>
      )}
    </div>
  )
}

export function Accessories() {
  const [q, setQ] = useState('')
  const parts = useGet<any[]>('/shop/parts' + qs({ q, limit: 60 }))
  const add = useAddToCart()
  return (
    <div>
      <Crumbs items={[{ label: 'لوازم جانبی و یدکی' }]} />
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3"><h1 className="text-xl font-extrabold">لوازم جانبی و یدکی</h1><input className="input max-w-xs" placeholder="جستجو…" value={q} onChange={(e) => setQ(e.target.value)} aria-label="جستجو" /></div>
      {parts.isLoading ? <Loading /> : !parts.data?.length ? <Empty title="موردی پیدا نشد" /> : (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-5">
          {parts.data.map((p) => (
            <Card key={p.id} className="flex flex-col overflow-hidden"><div className="aspect-square bg-muted"><Thumb src={p.image} name={p.name} /></div>
              <div className="flex flex-1 flex-col gap-2 p-3"><div className="line-clamp-2 min-h-[2.4rem] text-sm font-semibold">{p.name}{p.variant ? ` — ${p.variant}` : ''}</div>
                <div className="mt-auto flex items-end justify-between"><button className="btn-soft btn-sm !min-h-[34px] !px-2.5" aria-label="افزودن به سبد" onClick={() => add({ part_id: p.id })}><Plus className="h-4 w-4" /></button><span className="text-base font-extrabold num">{fa(p.price)} <span className="text-xs text-soft">تومان</span></span></div></div></Card>
          ))}
        </div>
      )}
    </div>
  )
}

export function ProductPage() {
  const { slug } = useParams()
  const q = useGet<any>(`/shop/products/${slug}`)
  const cart = useGet<any>('/shop/cart')
  const add = useAddToCart()
  const toast = useToast()
  const [img, setImg] = useState(0)
  const [qty, setQty] = useState(1)
  const [tab, setTab] = useState<'desc' | 'specs' | 'guarantee'>('desc')
  if (q.isLoading) return <Loading />
  if (q.error) return <Empty title="این محصول پیدا نشد" action={<Link className="btn-primary" to="/products">بازگشت به محصولات</Link>} />
  const p = q.data
  const orderable = !p.price_on_request && p.price > 0
  const specs = Object.entries(p.specs ?? {})
  const inCart = cart.data?.lines?.find((l: any) => l.product_id === p.id)?.qty ?? 0
  const buy = () => add({ product_id: p.id }, inCart + qty)
  const imgs: string[] = p.images?.length ? p.images : []
  return (
    <div className="space-y-8 pb-20 md:pb-0">
      <Crumbs items={[{ to: '/products', label: 'محصولات' }, ...(p.category_slug ? [{ to: `/products?category=${p.category_slug}`, label: p.category_name }] : []), { label: p.name }]} />
      <div className="grid gap-6 lg:grid-cols-[1.1fr_1fr_330px]">
        <div>
          <Card className="relative aspect-square overflow-hidden"><Thumb src={imgs[img]} name={p.name} />
            <div className="absolute left-3 top-3 flex flex-col gap-2"><WishHeart id={p.id} /><button aria-label="اشتراک‌گذاری" className="grid h-9 w-9 place-items-center rounded-full bg-surface/90 shadow" onClick={async () => { try { if (navigator.share) await navigator.share({ title: p.name, url: location.href }); else { await navigator.clipboard.writeText(location.href); toast.ok('پیوند کپی شد') } } catch { /* cancelled */ } }}><Share2 className="h-5 w-5 text-soft" /></button></div></Card>
          {imgs.length > 1 && <div className="mt-3 flex gap-2 overflow-x-auto">{imgs.map((s: string, i: number) => <button key={s} onClick={() => setImg(i)} className={clsx('h-16 w-16 shrink-0 overflow-hidden rounded-xl border-2', i === img ? 'border-brand' : 'border-transparent')}><img src={s} alt="" className="h-full w-full object-cover" /></button>)}</div>}
        </div>
        <div className="space-y-4">
          <div><h1 className="text-xl font-extrabold leading-9 md:text-2xl">{p.name}</h1>{p.name_en && <div className="text-sm text-soft" dir="ltr">{p.name_en}</div>}</div>
          {p.summary && <p className="text-sm leading-7 text-soft">{p.summary}</p>}
          {!!specs.length && <div><div className="mb-2 text-sm font-bold">ویژگی‌های کلیدی</div><ul className="space-y-1.5 text-sm">{specs.slice(0, 6).map(([k, v]) => <li key={k} className="flex items-start gap-2"><Check className="mt-0.5 h-4 w-4 shrink-0 text-brand" /><span><span className="text-soft">{k}:</span> <b>{String(v)}</b></span></li>)}</ul></div>}
        </div>
        <aside className="card h-fit space-y-4 p-5 lg:sticky lg:top-36">
          <div className="space-y-2.5 text-sm">
            <div className="flex items-center gap-2"><ShieldCheck className="h-5 w-5 text-ok" /><span>گارانتی {fa(p.guarantee_months)} ماهه اوینکی{p.golden_months > 0 && <b className="text-warn"> + {fa(p.golden_months)} ماه طلایی</b>}</span></div>
            <div className="flex items-center gap-2"><Truck className="h-5 w-5 text-soft" /><span>ارسال به سراسر کشور با کد رهگیری</span></div>
            <div className="flex items-center gap-2"><Wrench className="h-5 w-5 text-soft" /><span>خدمات پس از فروش در شهرهای بزرگ</span></div>
          </div>
          <div className="border-t border-line pt-4">
            {p.in_stock ? <Badge tone="ok">موجود در انبار</Badge> : <Badge tone="bad">ناموجود</Badge>}
            <div className="mt-3 flex items-end justify-between"><span className="text-xs text-soft">قیمت</span><Price price={p.price} onRequest={p.price_on_request} compare={p.compare_price} big /></div>
          </div>
          {orderable ? (
            <>
              <div className="flex items-center justify-between"><span className="text-sm text-soft">تعداد</span>
                <div className="flex items-center rounded-xl border border-line"><button className="p-3" aria-label="کم" onClick={() => setQty(Math.max(1, qty - 1))}><Minus className="h-4 w-4" /></button><span className="w-8 text-center font-bold num">{fa(qty)}</span><button className="p-3" aria-label="زیاد" onClick={() => setQty(Math.min(20, qty + 1))}><Plus className="h-4 w-4" /></button></div></div>
              <button className="btn-primary min-h-[50px] w-full text-base" disabled={!p.in_stock} onClick={buy}>{p.in_stock ? 'افزودن به سبد خرید' : 'ناموجود'}</button>
              {inCart > 0 && <Link to="/cart" className="btn-ghost w-full">{fa(inCart)} عدد در سبد شما — مشاهده سبد</Link>}
            </>
          ) : (
            <a className="btn-primary min-h-[50px] w-full text-base" href="tel:02833362824"><Phone className="h-5 w-5" />تماس برای قیمت و خرید</a>
          )}
        </aside>
      </div>

      <Card>
        <div className="flex gap-1 overflow-x-auto border-b border-line px-3" role="tablist">
          {([['desc', 'معرفی'], ['specs', 'مشخصات فنی'], ['guarantee', 'گارانتی و خدمات']] as const).map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)} className={clsx('whitespace-nowrap border-b-2 px-4 py-3.5 text-sm font-bold', tab === k ? 'border-brand text-brand' : 'border-transparent text-soft')}>{l}</button>)}
        </div>
        <div className="p-5">
          {tab === 'desc' && <p className="whitespace-pre-line text-sm leading-8">{p.description || p.summary || 'توضیحاتی ثبت نشده است.'}</p>}
          {tab === 'specs' && (specs.length ? <div className="divide-y divide-line">{specs.map(([k, v]) => <div key={k} className="grid grid-cols-3 gap-4 py-3 text-sm"><span className="text-soft">{k}</span><span className="col-span-2 font-medium">{String(v)}</span></div>)}</div> : <p className="text-sm text-soft">مشخصاتی ثبت نشده است.</p>)}
          {tab === 'guarantee' && (
            <div className="space-y-3 text-sm leading-8">
              <p>این محصول دارای <b>{fa(p.guarantee_months)} ماه گارانتی</b> شرکت اوینکی است{p.golden_months > 0 && <> و با فعال‌سازی گارانتی، <b>{fa(p.golden_months)} ماه گارانتی طلایی</b> به آن اضافه می‌شود</>}. تعمیر یا تعویض قطعات معیوب فنی در شرایط استفاده صحیح پوشش داده می‌شود.</p>
              <p className="text-soft">شامل گارانتی نمی‌شود: آسیب فیزیکی، نصب یا استفاده نادرست، ولتاژ غیرمجاز، بازکردن دستگاه توسط افراد غیرمجاز و پایان مدت گارانتی.</p>
              <Link to="/guarantee" className="btn-soft">ثبت و استعلام گارانتی</Link>
            </div>
          )}
        </div>
      </Card>

      {!!p.accessories?.length && <Row title="لوازم جانبی این محصول" to="/accessories">{p.accessories.map((a: any) => <AccessoryCard key={a.id} a={a} />)}</Row>}
      {!!p.related?.length && <Row title="محصولات مرتبط">{p.related.map((r: any) => <ProductCard key={r.id} p={r} compact />)}</Row>}

      {/* phones: sticky buy bar */}
      {orderable && (
        <div className="fixed inset-x-0 bottom-[calc(58px+env(safe-area-inset-bottom))] z-30 flex items-center gap-3 border-t border-line bg-surface p-3 md:hidden">
          <button className="btn-primary flex-1" disabled={!p.in_stock} onClick={buy}>{p.in_stock ? 'افزودن به سبد' : 'ناموجود'}</button><Price price={p.price} compare={p.compare_price} />
        </div>
      )}
    </div>
  )
}

function AccessoryCard({ a }: { a: any }) {
  const add = useAddToCart()
  return (
    <Card className="w-44 shrink-0 snap-start overflow-hidden sm:w-56"><div className="aspect-square bg-muted"><Thumb src={a.image} name={a.name} /></div>
      <div className="p-3"><div className="line-clamp-2 min-h-[2.4rem] text-sm font-semibold">{a.name}</div><div className="mt-2 flex items-end justify-between"><button className="btn-soft btn-sm !min-h-[34px] !px-2.5" aria-label="افزودن" onClick={() => add({ part_id: a.id })}><Plus className="h-4 w-4" /></button><span className="font-extrabold num">{fa(a.price)}</span></div></div></Card>
  )
}

void toman
