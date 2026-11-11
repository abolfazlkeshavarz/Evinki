import { useState } from 'react'
import { Badge } from '../../components/ui'
import { FieldDef, ResourcePage } from '../../components/Resource'
import { CodeGenerator } from '../../components/CodeFormula'
import { ActionButton, Card, Modal } from '../../components/ui'
import { del, post, put, get } from '../../lib/api'
import { date, fa, toman } from '../../lib/format'
import { useGet, useInvalidate } from '../../lib/hooks'

const yesNo = (v: boolean) => <Badge tone={v ? 'ok' : 'mute'}>{v ? 'فعال' : 'غیرفعال'}</Badge>

export function Categories() {
  const parents = useGet<any>('/admin/categories?limit=200')
  const fields: FieldDef[] = [
    { key: 'name', label: 'نام', required: true, half: true }, { key: 'slug', label: 'شناسه در آدرس (انگلیسی)', required: true, half: true, hint: 'مثال: vacuum-cleaners' },
    { key: 'name_en', label: 'نام انگلیسی', half: true }, { key: 'parent_id', label: 'دسته والد', type: 'select', half: true, options: parents.data?.items.map((c: any) => ({ value: c.id, label: c.name })) },
    { key: 'sort_order', label: 'ترتیب نمایش', type: 'int', half: true }, { key: 'is_active', label: 'وضعیت', type: 'bool', half: true }, { key: 'description', label: 'توضیح', type: 'textarea' },
  ]
  return <ResourcePage title="دسته‌بندی محصولات" path="/admin/categories" fields={fields} canWrite={['catalog.manage']} canDelete={['catalog.delete']} newRow={{ is_active: true, sort_order: 0 }}
    columns={[{ head: 'نام', cell: (r) => <b>{r.name}</b> }, { head: 'شناسه', cell: (r) => <span dir="ltr" className="text-soft">{r.slug}</span> }, { head: 'محصولات', cell: (r) => fa(r.product_count) }, { head: 'وضعیت', cell: (r) => yesNo(r.is_active) }]} />
}

export function AdminProducts() {
  const cats = useGet<any>('/admin/categories?limit=200')
  const fields: FieldDef[] = [
    { key: 'name', label: 'نام محصول', required: true, half: true }, { key: 'name_en', label: 'نام انگلیسی', half: true },
    { key: 'sku', label: 'کد کالا (SKU)', required: true, half: true }, { key: 'slug', label: 'شناسه در آدرس', required: true, half: true, hint: 'فقط حروف انگلیسی و خط تیره' },
    { key: 'category_id', label: 'دسته‌بندی', type: 'select', half: true, options: cats.data?.items.map((c: any) => ({ value: c.id, label: c.name })) },
    { key: 'stock_qty', label: 'موجودی انبار محصول', type: 'int', half: true },
    { key: 'price', label: 'قیمت (تومان)', type: 'money', half: true }, { key: 'compare_price', label: 'قیمت قبل از تخفیف', type: 'money', half: true },
    { key: 'price_on_request', label: 'قیمت با تماس (غیرقابل خرید آنلاین)', type: 'bool', half: true }, { key: 'is_published', label: 'نمایش در فروشگاه', type: 'bool', half: true },
    { key: 'summary', label: 'خلاصه', type: 'textarea' }, { key: 'description', label: 'توضیحات کامل', type: 'textarea' },
    { key: 'images', label: 'تصاویر', type: 'images' }, { key: 'specs', label: 'مشخصات فنی', type: 'kv' },
    { key: 'guarantee_months', label: 'مدت گارانتی (ماه)', type: 'int', half: true }, { key: 'golden_months', label: 'گارانتی طلایی (ماه)', type: 'int', half: true },
    { key: 'code_format', label: 'قالب کد گارانتی', type: 'select', half: true, options: [{ value: 'simple', label: 'ساده' }, { value: 'evinki_vacuum', label: 'اوینکی (سال+پیشوند+ماه+سریال، با کدهای فصلی قدیم)' }, { value: 'jalali_encoded', label: 'رمزگذاری‌شده شمسی' }, { value: 'formula', label: 'فرمول (ساخته‌شده با سازنده زیر)' }, { value: 'any', label: 'هر کدی' }] },
    { key: 'code_prefix', label: 'پیشوند کد گارانتی', half: true, hint: 'مثلاً FZ' }, { key: 'code_pattern', label: 'الگوی دلخواه (Regex)', half: true },
    { key: 'warranty_basis', label: 'شروع محاسبه گارانتی از', type: 'select', hint: 'تاریخ تولید از روی کد خوانده می‌شود؛ اگر کد تاریخ نداشته باشد، از تاریخ خرید حساب می‌شود.', options: [{ value: 'manufacture', label: 'تاریخ تولید (پیش‌فرض)' }, { value: 'purchase', label: 'تاریخ خرید' }, { value: 'best', label: 'بهترین حالت برای مشتری (دیرتر از این دو)' }] },
    { key: 'code_formula', label: 'فرمول ساخت کد گارانتی', type: 'formula', hint: 'اگر فرمول بسازید، جایگزین قالب و پیشوند بالا می‌شود.' },
  ]
  const [gen, setGen] = useState<any>(null)
  const [merge, setMerge] = useState<any>(null)
  return <><ResourcePage title="محصولات" path="/admin/products" rowActions={(r) => <>{r.code_formula?.length > 0 && <button className="btn-soft btn-sm" onClick={() => setGen(r)}>تولید کد</button>}<button className="btn-ghost btn-sm" onClick={() => setMerge(r)}>ادغام</button></>} fields={fields} canWrite={['catalog.manage']} canDelete={['catalog.delete']} newRow={{ is_published: true, guarantee_months: 18, golden_months: 3, code_format: 'simple', warranty_basis: 'manufacture', code_formula: [], images: [], specs: {}, price: 0, stock_qty: 0 }}
    columns={[{ head: 'محصول', cell: (r) => <div><b>{r.name}</b><div className="text-xs text-soft" dir="ltr">{r.sku}</div></div> }, { head: 'دسته', cell: (r) => r.category_name || '—' },
      { head: 'قیمت', cell: (r) => (r.price_on_request || !r.price ? <Badge>تماس بگیرید</Badge> : <span className="num">{toman(r.price)}</span>) }, { head: 'موجودی', cell: (r) => fa(r.stock_qty) },
      { head: 'گارانتی', cell: (r) => `${fa(r.guarantee_months)}+${fa(r.golden_months)} ماه` }, { head: 'فروشگاه', cell: (r) => yesNo(r.is_published) }]} />
    <CodeGenerator product={gen} onClose={() => setGen(null)} />
    <MergeProduct product={merge} onClose={() => setMerge(null)} />
  </>
}

/** Two entries for the same device (e.g. the shop's FZ530 and the guarantee system's FZ530): everything moves to the one you keep. */
function MergeProduct({ product, onClose }: { product: any; onClose: () => void }) {
  const all = useGet<any>(product ? '/admin/products?limit=500' : null)
  const inv = useInvalidate()
  const [into, setInto] = useState(0)
  return (
    <Modal open={!!product} onClose={onClose} title={`ادغام «${product?.name ?? ''}» در محصول دیگر`} footer={<ActionButton className="btn-primary" disabled={!into} confirm="همه گارانتی‌ها، تعمیرها و سفارش‌های این محصول به محصول مقصد منتقل و این محصول حذف می‌شود. ادامه می‌دهید؟" onClick={async () => { await post(`/admin/products/${product.id}/merge`, { into }); inv('/admin'); setInto(0); onClose() }}>ادغام</ActionButton>}>
      <p className="mb-3 text-sm leading-7 text-soft">محصول مقصد را انتخاب کنید. گارانتی‌ها، تعمیرها، سفارش‌ها و سبدها به آن منتقل می‌شوند؛ اطلاعات خالی مقصد (عکس، مشخصات، قالب کد) از این محصول پر می‌شود و این محصول بایگانی می‌شود.</p>
      <select className="input" value={into} onChange={(e) => setInto(+e.target.value)}><option value={0}>انتخاب محصول مقصد…</option>{all.data?.items.filter((p: any) => p.id !== product?.id).map((p: any) => <option key={p.id} value={p.id}>{p.name} ({p.sku})</option>)}</select>
    </Modal>
  )
}

export function Parts() {
  const [compat, setCompat] = useState<any>(null)
  const [sel, setSel] = useState<number[]>([])
  const products = useGet<any[]>('/lookup/products')
  const inv = useInvalidate()
  const fields: FieldDef[] = [
    { key: 'name', label: 'نام قطعه', required: true, half: true }, { key: 'code', label: 'کد قطعه', required: true, half: true },
    { key: 'variant', label: 'حالت / مدل', half: true }, { key: 'part_no', label: 'شماره قطعه', half: true },
    { key: 'sepidar_code', label: 'کد کالا در سپیدار', half: true }, { key: 'unit', label: 'واحد', half: true },
    { key: 'material', label: 'جنس', half: true }, { key: 'location', label: 'محل در انبار', half: true },
    { key: 'min_stock', label: 'حداقل موجودی (هشدار)', type: 'int', half: true }, { key: 'image', label: 'تصویر', type: 'image', half: true },
    { key: 'buyback_price', label: 'قیمت خرید قطعه قدیمی از تکنسین', type: 'money', half: true, hint: 'در صورتحساب مرسوله خودکار استفاده می‌شود' },
    { key: 'sale_price', label: 'قیمت فروش در فروشگاه', type: 'money', half: true },
    { key: 'is_sellable', label: 'قابل فروش آنلاین', type: 'bool', half: true }, { key: 'return_required', label: 'قطعه قدیمی باید برگردد', type: 'bool', half: true },
    { key: 'is_active', label: 'وضعیت', type: 'bool', half: true }, { key: 'note', label: 'توضیح', type: 'textarea' },
  ]
  return (
    <>
      <ResourcePage title="کاتالوگ قطعات" subtitle="همین فهرست برای انبار، درخواست تکنسین و فروشگاه استفاده می‌شود." path="/admin/parts" fields={fields} canWrite={['catalog.manage']} canDelete={['catalog.delete']}
        newRow={{ is_active: true, return_required: true, unit: 'عدد', min_stock: 0, buyback_price: 0, sale_price: 0 }} pageSize={30}
        rowActions={(r) => <button className="btn-ghost btn-sm" onClick={async () => { setSel((await get<any>(`/admin/parts/${r.id}/compat`)).product_ids); setCompat(r) }}>سازگاری</button>}
        columns={[{ head: 'قطعه', cell: (r) => <div><b>{r.name}</b>{r.variant && <span className="text-soft"> — {r.variant}</span>}<div className="text-xs text-soft" dir="ltr">{r.code}</div></div> },
          { head: 'موجودی', cell: (r) => <span className={r.stock <= r.min_stock ? 'font-bold text-bad num' : 'num'}>{fa(r.stock)}</span> },
          { head: 'قیمت خرید قدیمی', cell: (r) => (r.buyback_price ? <span className="num">{toman(r.buyback_price)}</span> : '—') },
          { head: 'فروش آنلاین', cell: (r) => (r.is_sellable ? <span className="num">{toman(r.sale_price)}</span> : '—') }, { head: 'وضعیت', cell: (r) => yesNo(r.is_active) }]} />
      <Modal open={!!compat} onClose={() => setCompat(null)} title={`سازگاری: ${compat?.name ?? ''}`} footer={<ActionButton onClick={async () => { await put(`/admin/parts/${compat.id}/compat`, { product_ids: sel }); inv('/admin/parts'); setCompat(null) }}>ذخیره</ActionButton>}>
        <p className="mb-3 text-sm text-soft">این قطعه برای کدام محصولات است؟ (در صفحه محصول به‌عنوان لوازم جانبی نمایش داده می‌شود.)</p>
        <div className="space-y-2">{products.data?.map((p) => <label key={p.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={sel.includes(p.id)} onChange={(e) => setSel(e.target.checked ? [...sel, p.id] : sel.filter((x) => x !== p.id))} />{p.name}</label>)}</div>
      </Modal>
    </>
  )
}

export function Coupons() {
  const q = useGet<any[]>('/admin/coupons')
  const inv = useInvalidate()
  const [e, setE] = useState<any>(null)
  return (
    <div>
      <div className="mb-5 flex items-center justify-between"><h1 className="text-xl font-bold md:text-2xl">کد تخفیف</h1><button className="btn-primary" onClick={() => setE({ code: '', kind: 'percent', value: 10, max_discount: 0, min_total: 0, max_uses: 0, is_active: true })}>کد جدید</button></div>
      <Card className="divide-y divide-line">
        {q.data?.length === 0 && <div className="p-8 text-center text-soft">کدی ثبت نشده است</div>}
        {q.data?.map((c) => (
          <div key={c.code} className="flex flex-wrap items-center justify-between gap-3 p-4 text-sm">
            <div><b dir="ltr" className="text-base">{c.code}</b> {yesNo(c.is_active)}<div className="text-xs text-soft">{c.kind === 'percent' ? `${fa(c.value)}٪` : toman(c.value)} · استفاده: {fa(c.used_count)}{c.max_uses ? ` از ${fa(c.max_uses)}` : ''}{c.ends_at ? ` · تا ${date(c.ends_at)}` : ''}</div></div>
            <div className="flex gap-2"><button className="btn-ghost btn-sm" onClick={() => setE(c)}>ویرایش</button><ActionButton className="btn-ghost btn-sm text-bad" confirm="حذف شود؟" onClick={async () => { await del(`/admin/coupons/${c.code}`); inv('/admin/coupons') }}>حذف</ActionButton></div>
          </div>
        ))}
      </Card>
      <Modal open={!!e} onClose={() => setE(null)} title="کد تخفیف" footer={<ActionButton onClick={async () => { await post('/admin/coupons', { ...e, starts_at: e.starts_at || null, ends_at: e.ends_at || null }); inv('/admin/coupons'); setE(null) }}>ذخیره</ActionButton>}>
        {e && <div className="grid gap-3 sm:grid-cols-2">
          <label className="sm:col-span-2"><span className="label">کد</span><input className="input" dir="ltr" value={e.code} onChange={(x) => setE({ ...e, code: x.target.value.toUpperCase() })} /></label>
          <label><span className="label">نوع</span><select className="input" value={e.kind} onChange={(x) => setE({ ...e, kind: x.target.value })}><option value="percent">درصدی</option><option value="fixed">مبلغ ثابت</option></select></label>
          <label><span className="label">مقدار {e.kind === 'percent' ? '(درصد)' : '(تومان)'}</span><input className="input num" type="number" value={e.value} onChange={(x) => setE({ ...e, value: +x.target.value })} /></label>
          <label><span className="label">سقف تخفیف (تومان، ۰ = بدون سقف)</span><input className="input num" type="number" value={e.max_discount} onChange={(x) => setE({ ...e, max_discount: +x.target.value })} /></label>
          <label><span className="label">حداقل خرید</span><input className="input num" type="number" value={e.min_total} onChange={(x) => setE({ ...e, min_total: +x.target.value })} /></label>
          <label><span className="label">حداکثر دفعات استفاده (۰ = نامحدود)</span><input className="input num" type="number" value={e.max_uses} onChange={(x) => setE({ ...e, max_uses: +x.target.value })} /></label>
          <label className="flex items-center gap-2 pt-6 text-sm"><input type="checkbox" checked={e.is_active} onChange={(x) => setE({ ...e, is_active: x.target.checked })} />فعال</label>
        </div>}
      </Modal>
    </div>
  )
}
