import { useState } from 'react'
import { Plus } from 'lucide-react'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, Loading, Modal, MoneyInput, PageHeader, Pager, SearchBox, useToast } from '../../components/ui'
import { del, patch, post, put } from '../../lib/api'
import { useUser } from '../../lib/auth'
import { dateTime, fa } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { roleTitle } from '../../lib/labels'

export function Users() {
  const [q, setQ] = useState('')
  const [role, setRole] = useState('')
  const [page, setPage] = useState(1)
  const [edit, setEdit] = useState<any>(null)
  const [pw, setPw] = useState<any>(null)
  const me = useUser()
  const inv = useInvalidate()
  const roles = useGet<any[]>('/admin/roles')
  const list = useGet<any>('/admin/users' + qs({ q, role, page, limit: 25 }))
  const canSuper = me?.roles.includes('super_admin')
  const assignable = (roles.data ?? []).filter((r) => !['customer'].includes(r.code) && (canSuper || !['super_admin', 'admin'].includes(r.code)))
  const save = async () => {
    if (edit.id) { await patch(`/admin/users/${edit.id}`, { full_name: edit.full_name, phone: edit.phone, email: edit.email, is_active: edit.is_active }); await put(`/admin/users/${edit.id}/roles`, { roles: edit.roles }) }
    else await post('/admin/users', edit)
    inv('/admin/users'); setEdit(null)
  }
  return (
    <div>
      <PageHeader title="کاربران" actions={<button className="btn-primary" onClick={() => setEdit({ username: '', full_name: '', phone: '', password: '', roles: [], is_active: true })}><Plus className="h-4 w-4" />کاربر جدید</button>} />
      <Card>
        <div className="flex flex-wrap gap-3 border-b border-line p-3"><SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} /><select className="input w-auto" value={role} onChange={(e) => { setRole(e.target.value); setPage(1) }}><option value="">همه نقش‌ها</option>{roles.data?.map((r) => <option key={r.code} value={r.code}>{roleTitle[r.code] ?? r.title_fa}</option>)}</select></div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty /> : (
          <><DataTable rows={list.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => setEdit({ ...r })} cols={[
            { head: 'کاربر', cell: (r: any) => <div><b>{r.full_name || r.username}</b><div className="text-xs text-soft" dir="ltr">{r.username}</div></div> }, { head: 'نقش‌ها', cell: (r: any) => <div className="flex flex-wrap gap-1">{r.roles.map((x: string) => <Badge key={x} tone="info">{roleTitle[x] ?? x}</Badge>)}</div> },
            { head: 'آخرین ورود', cell: (r: any) => (r.last_login_at ? dateTime(r.last_login_at) : '—') }, { head: 'وضعیت', cell: (r: any) => <Badge tone={r.is_active ? 'ok' : 'mute'}>{r.is_active ? 'فعال' : 'غیرفعال'}</Badge> }]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} /></>
        )}
      </Card>
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit?.id ? 'ویرایش کاربر' : 'کاربر جدید'} wide footer={<>{edit?.id && <><button className="btn-ghost" onClick={() => setPw({ id: edit.id, password: '', must_change: true })}>تغییر گذرواژه</button>{edit.id !== me?.id && <ActionButton className="btn-ghost text-bad" confirm="کاربر حذف شود؟" onClick={async () => { await del(`/admin/users/${edit.id}`); inv('/admin/users'); setEdit(null) }}>حذف</ActionButton>}</>}<ActionButton onClick={save}>ذخیره</ActionButton></>}>
        {edit && <div className="grid gap-4 sm:grid-cols-2">
          <Field label="نام کاربری"><input className="input" dir="ltr" disabled={!!edit.id} value={edit.username} onChange={(e) => setEdit({ ...edit, username: e.target.value })} /></Field>
          <Field label="نام کامل"><input className="input" value={edit.full_name} onChange={(e) => setEdit({ ...edit, full_name: e.target.value })} /></Field>
          <Field label="موبایل"><input className="input" dir="ltr" value={edit.phone} onChange={(e) => setEdit({ ...edit, phone: e.target.value })} /></Field>
          {!edit.id && <Field label="گذرواژه اولیه" hint="کاربر در اولین ورود باید آن را تغییر دهد."><input className="input" dir="ltr" value={edit.password} onChange={(e) => setEdit({ ...edit, password: e.target.value })} /></Field>}
          <Field label="نقش‌ها" className="sm:col-span-2"><div className="flex flex-wrap gap-2">{assignable.map((r) => {
            const on = edit.roles.includes(r.code)
            return <button key={r.code} type="button" aria-pressed={on} onClick={() => setEdit({ ...edit, roles: on ? edit.roles.filter((x: string) => x !== r.code) : [...edit.roles, r.code] })} className={`rounded-xl border px-3 py-2 text-sm font-medium ${on ? 'border-brand bg-brand-tint text-brand' : 'border-line'}`}>{roleTitle[r.code] ?? r.title_fa}</button>
          })}</div></Field>
          {edit.id && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={edit.is_active} onChange={(e) => setEdit({ ...edit, is_active: e.target.checked })} />حساب فعال است</label>}
        </div>}
      </Modal>
      <Modal open={!!pw} onClose={() => setPw(null)} title="تعیین گذرواژه" footer={<ActionButton onClick={async () => { await post(`/admin/users/${pw.id}/reset-password`, pw); setPw(null) }} success="گذرواژه تغییر کرد؛ کاربر از همه دستگاه‌ها خارج شد">ذخیره</ActionButton>}>
        {pw && <div className="space-y-3"><Field label="گذرواژه جدید (حداقل ۸ نویسه)"><input className="input" dir="ltr" value={pw.password} onChange={(e) => setPw({ ...pw, password: e.target.value })} /></Field><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={pw.must_change} onChange={(e) => setPw({ ...pw, must_change: e.target.checked })} />در ورود بعدی تغییر دهد</label></div>}
      </Modal>
    </div>
  )
}

export function Roles() {
  const roles = useGet<any[]>('/admin/roles')
  const perms = useGet<{ key: string; title: string }[]>('/admin/permissions')
  const inv = useInvalidate()
  const [sel, setSel] = useState<string>('')
  const [draft, setDraft] = useState<string[] | null>(null)
  if (roles.isLoading || perms.isLoading) return <Loading />
  const cur = roles.data?.find((r) => r.code === (sel || roles.data![0].code))!
  const active = draft ?? cur.permissions
  const locked = cur.code === 'super_admin'
  return (
    <div>
      <PageHeader title="نقش‌ها و دسترسی‌ها" subtitle="هر نقش مجموعه‌ای از دسترسی‌هاست. تغییرات بلافاصله اعمال می‌شود." />
      <div className="grid gap-4 md:grid-cols-[240px_1fr]">
        <Card className="h-fit p-2">{roles.data!.map((r) => <button key={r.code} onClick={() => { setSel(r.code); setDraft(null) }} className={`flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-start text-sm ${cur.code === r.code ? 'bg-brand-tint font-semibold text-brand' : 'hover:bg-muted'}`}><span>{roleTitle[r.code] ?? r.title_fa}</span><span className="text-xs text-soft num">{fa(r.users)} کاربر</span></button>)}</Card>
        <Card className="p-4">
          {locked ? <p className="text-sm text-soft">مدیر ارشد به همه چیز دسترسی دارد و قابل تغییر نیست.</p> : (
            <>
              <div className="grid gap-1 sm:grid-cols-2">{perms.data!.map((p) => <label key={p.key} className="flex items-start gap-2 rounded-lg p-2 text-sm hover:bg-muted"><input type="checkbox" className="mt-1" checked={active.includes(p.key)} onChange={(e) => setDraft(e.target.checked ? [...active, p.key] : active.filter((x: string) => x !== p.key))} /><span>{p.title}<span className="block text-[11px] text-soft" dir="ltr">{p.key}</span></span></label>)}</div>
              <ActionButton className="btn-primary mt-4" disabled={!draft} onClick={async () => { await put(`/admin/roles/${cur.code}/permissions`, { permissions: draft }); inv('/admin/roles'); setDraft(null) }} success="ذخیره شد">ذخیره دسترسی‌ها</ActionButton>
            </>
          )}
        </Card>
      </div>
    </div>
  )
}

export function Sms() {
  const t = useGet<any[]>('/admin/sms/templates')
  const out = useGet<any[]>('/admin/sms/outbox')
  const inv = useInvalidate()
  const [e, setE] = useState<any>(null)
  const [phone, setPhone] = useState('')
  const toast = useToast()
  return (
    <div className="space-y-6">
      <PageHeader title="الگوهای پیامک" subtitle="شناسه الگو را از پنل ملی‌پیامک کپی کنید. الگوی بدون شناسه ارسال نمی‌شود." actions={<button className="btn-primary" onClick={() => setE({ key: '', title: '', description: '', body_id: 0, sample_text: '', is_active: true, isNew: true })}><Plus className="h-4 w-4" />الگوی جدید</button>} />
      <Card>{t.isLoading ? <Loading /> : t.data?.map((x) => (
        <div key={x.key} className="flex flex-wrap items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0">
          <div><b>{x.title}</b> <span className="text-xs text-soft" dir="ltr">{x.key}</span> {x.body_id ? <Badge tone="ok">شناسه {fa(x.body_id)}</Badge> : <Badge tone="warn">تنظیم نشده</Badge>}<div className="text-xs text-soft">{x.description}</div></div>
          <button className="btn-ghost btn-sm" onClick={() => setE(x)}>ویرایش</button>
        </div>))}</Card>
      <Card className="flex flex-wrap items-end gap-3 p-4"><Field label="ارسال پیامک آزمایشی به"><input className="input" dir="ltr" placeholder="09123456789" value={phone} onChange={(e) => setPhone(e.target.value)} /></Field><ActionButton className="btn-soft" onClick={async () => { await post('/admin/sms/test', { phone }); toast.ok('در صف ارسال قرار گرفت'); inv('/admin/sms/outbox') }}>ارسال</ActionButton></Card>
      <Card><div className="border-b border-line p-4 font-bold">آخرین پیامک‌ها</div>{out.data?.slice(0, 20).map((o) => <div key={o.id} className="flex items-center justify-between gap-3 border-b border-line/60 px-4 py-2.5 text-sm last:border-0"><span dir="ltr" className="num">{o.phone}</span><span className="text-soft">{o.template_key}</span><span className="text-xs">{o.error && <span className="text-bad">{o.error} · </span>}{dateTime(o.created_at)}</span><Badge tone={{ sent: 'ok', pending: 'warn', failed: 'bad', skipped: 'mute' }[o.status as string] as any}>{{ sent: 'ارسال شد', pending: 'در صف', failed: 'ناموفق', skipped: 'رد شد' }[o.status as string]}</Badge></div>)}</Card>
      <Modal open={!!e} onClose={() => setE(null)} title="الگوی پیامک" footer={<><ActionButton className="btn-ghost text-bad" disabled={!e || !!e.is_builtin || !!e.isNew} onClick={async () => { await del(`/admin/sms/templates/${e.key}`); inv('/admin/sms'); setE(null) }}>حذف</ActionButton><ActionButton onClick={async () => { await put(`/admin/sms/templates/${e.key}`, { title: e.title, description: e.description, body_id: +e.body_id || 0, sample_text: e.sample_text, is_active: e.is_active }); inv('/admin/sms'); setE(null) }}>ذخیره</ActionButton></>}>
        {e && <div className="space-y-3"><Field label="کلید"><input className="input" dir="ltr" disabled={!e.isNew} value={e.key} onChange={(x) => setE({ ...e, key: x.target.value })} /></Field><Field label="عنوان"><input className="input" value={e.title} onChange={(x) => setE({ ...e, title: x.target.value })} /></Field>
          <Field label="شناسه الگو در ملی‌پیامک (bodyId)"><input className="input num" dir="ltr" value={e.body_id} onChange={(x) => setE({ ...e, body_id: x.target.value.replace(/\D/g, '') })} /></Field>
          <Field label="متن نمونه (فقط برای یادآوری)"><textarea className="input" rows={3} value={e.sample_text} onChange={(x) => setE({ ...e, sample_text: x.target.value })} /></Field><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={e.is_active} onChange={(x) => setE({ ...e, is_active: x.target.checked })} />فعال</label></div>}
      </Modal>
    </div>
  )
}

export function SettingsPage() {
  const s = useGet<Record<string, any>>('/admin/settings')
  const [d, setD] = useState<Record<string, any>>({})
  const inv = useInvalidate()
  if (s.isLoading) return <Loading />
  if (s.error) return <ErrorBox error={s.error} />
  const v = { ...s.data!, ...d }
  const set = (k: string, val: any) => setD({ ...d, [k]: val })
  const Toggle = ({ k, label, hint }: { k: string; label: string; hint?: string }) => <label className="flex items-start gap-3 py-2 text-sm"><input type="checkbox" className="mt-1 h-4 w-4" checked={!!v[k]} onChange={(e) => set(k, e.target.checked)} /><span>{label}{hint && <span className="block text-xs text-soft">{hint}</span>}</span></label>
  return (
    <div className="max-w-2xl space-y-5">
      <PageHeader title="تنظیمات" />
      <Card className="space-y-4 p-4"><h2 className="font-bold">عمومی</h2>
        <div className="grid gap-3 sm:grid-cols-2"><Field label="نام شرکت"><input className="input" value={v.company_name} onChange={(e) => set('company_name', e.target.value)} /></Field><Field label="تلفن"><input className="input" dir="ltr" value={v.company_phone} onChange={(e) => set('company_phone', e.target.value)} /></Field></div></Card>
      <Card className="space-y-4 p-4"><h2 className="font-bold">فروشگاه</h2>
        <div className="grid gap-3 sm:grid-cols-2"><Field label="هزینه ارسال (تومان)"><MoneyInput value={v.shipping_fee} onChange={(n) => set('shipping_fee', n)} /></Field><Field label="ارسال رایگان بالاتر از (۰ = هرگز)"><MoneyInput value={v.free_shipping_over} onChange={(n) => set('free_shipping_over', n)} /></Field></div>
        <Toggle k="guarantee_auto_approve_orders" label="گارانتی دستگاه‌های خریداری‌شده از فروشگاه پس از تحویل خودکار فعال شود" hint="مشروط به ثبت سریال دستگاه روی سفارش" /></Card>
      <Card className="space-y-1 p-4"><h2 className="mb-1 font-bold">انبار</h2>
        <Toggle k="wh_check_stock" label="خروج بیش از موجودی مجاز نباشد" /><Toggle k="wh_approval_out" label="خروج‌های انباردار نیاز به تایید مدیر داشته باشد" /><Toggle k="wh_approval_in" label="ورودی‌های انباردار نیاز به تایید مدیر داشته باشد" /><Toggle k="wh_count_blind" label="در انبارگردانی موجودی سیستم به انباردار نمایش داده نشود" /></Card>
      <Card className="space-y-1 p-4"><h2 className="mb-1 font-bold">خدمات و نظرسنجی</h2><Toggle k="service_auto_survey" label="پس از هر تعمیر تایید‌شده نظرسنجی خودکار ارسال شود" hint="نیازمند نظرسنجی فعال با ماشه «پس از تعمیر»" /></Card>
      <Card className="space-y-1 p-4"><h2 className="mb-1 font-bold">حضور و غیاب</h2><Toggle k="hr_face_enabled" label="ورود با تشخیص چهره" /><Toggle k="hr_pin_enabled" label="ورود با کد پرسنلی و رمز" /><Toggle k="hr_fingerprint_enabled" label="ورود با اثر انگشت" /><Toggle k="hr_liveness" label="تشخیص زنده بودن (ضد عکس)" /></Card>
      <ActionButton className="btn-primary" disabled={!Object.keys(d).length} success="تنظیمات ذخیره شد" onClick={async () => { await put('/admin/settings', d); setD({}); inv('/admin/settings') }}>ذخیره تنظیمات</ActionButton>
    </div>
  )
}

export function Audit() {
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const d = useGet<any>('/admin/audit' + qs({ q, page, limit: 50 }))
  return (
    <div><PageHeader title="گزارش ممیزی" subtitle="هر تغییر در سامانه همراه با کاربر و زمان ثبت می‌شود و قابل ویرایش یا حذف نیست." />
      <Card><div className="border-b border-line p-3"><SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="جستجو در رویدادها" /></div>
        {d.isLoading ? <Loading /> : d.error ? <ErrorBox error={d.error} /> : <><DataTable rows={d.data.items} rowKey={(r: any) => r.id} cols={[{ head: 'زمان', cell: (r: any) => <span className="text-xs">{dateTime(r.at)}</span> }, { head: 'کاربر', cell: (r: any) => r.actor_name || 'سیستم' }, { head: 'رویداد', cell: (r: any) => <span dir="ltr" className="text-xs">{r.action}</span> }, { head: 'مورد', cell: (r: any) => <span dir="ltr" className="text-xs">{r.entity} {r.entity_id}</span> }, { head: 'جزئیات', cell: (r: any) => <span className="text-xs text-soft">{r.detail}</span> }, { head: 'IP', cell: (r: any) => <span dir="ltr" className="text-xs text-soft">{r.ip}</span> }]} /><Pager page={page} total={d.data.total} limit={d.data.limit} onPage={setPage} /></>}
      </Card>
    </div>
  )
}
