import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowDown, ArrowUp, Plus, Star, Trash2 } from 'lucide-react'
import { ActionButton, Badge, Card, Empty, ErrorBox, Field, Loading, PageHeader, Stat, StatusBadge, useToast } from '../../components/ui'
import { del, post, put } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { fa } from '../../lib/format'
import { useGet, useInvalidate } from '../../lib/hooks'
import { surveyStatus } from '../../lib/labels'

const triggerLabel: Record<string, string> = { manual: 'ارسال دستی به مشتریان', repair_approved: 'خودکار پس از هر تعمیر تایید‌شده', order_delivered: 'خودکار پس از تحویل سفارش' }
const kindLabel: Record<string, string> = { rating: 'امتیاز ۱ تا ۵', nps: 'احتمال پیشنهاد (۰ تا ۱۰)', yes_no: 'بله / خیر', choice: 'چندگزینه‌ای', text: 'متن آزاد' }

export function Surveys() {
  const d = useGet<any[]>('/admin/surveys')
  const can = useCan()
  return (
    <div>
      <PageHeader title="نظرسنجی‌ها" subtitle="نظرسنجی خودکار بعد از هر سرویس، بدون هیچ کار دستی، رضایت مشتری از هر تکنسین را اندازه می‌گیرد." actions={can('surveys.manage') && <Link to="/admin/surveys/new" className="btn-primary"><Plus className="h-4 w-4" />نظرسنجی جدید</Link>} />
      <div className="grid gap-3 md:grid-cols-2">
        {d.isLoading ? <Loading /> : !d.data?.length ? <div className="md:col-span-2"><Empty title="نظرسنجی‌ای ساخته نشده" /></div> : d.data.map((s) => (
          <Link key={s.id} to={`/admin/surveys/${s.id}`} className="card p-4 transition hover:border-brand">
            <div className="flex items-start justify-between gap-2"><b>{s.title}</b><StatusBadge map={surveyStatus} value={s.status} /></div>
            <div className="mt-1 text-xs text-soft">{triggerLabel[s.trigger]}</div>
            <div className="mt-3 flex gap-4 text-sm"><span>{fa(s.recipients)} دعوت</span><span>{fa(s.responses)} پاسخ</span>{s.recipients > 0 && <span className="text-ok">{fa(Math.round((s.responses * 100) / s.recipients))}٪</span>}</div>
          </Link>
        ))}
      </div>
    </div>
  )
}

export function SurveyEditor() {
  const { id } = useParams()
  const nav = useNavigate()
  const inv = useInvalidate()
  const existing = useGet<any>(id ? `/admin/surveys/${id}` : null)
  const [f, setF] = useState<any>(null)
  const sms = useGet<any[]>('/admin/sms/templates')
  if (id && existing.isLoading) return <Loading />
  const v = f ?? (id ? existing.data : { title: '', description: '', trigger: 'repair_approved', sms_template: 'survey_invite', offer_template: '', link_mode: 'url', filter: {}, questions: [{ text: 'از خدمات ما چقدر راضی بودید؟', kind: 'rating', options: [], required: true }] })
  if (!v) return <ErrorBox error={existing.error} />
  const set = (p: any) => setF({ ...v, ...p })
  const setQ = (i: number, p: any) => set({ questions: v.questions.map((q: any, j: number) => (j === i ? { ...q, ...p } : q)) })
  const move = (i: number, d: number) => { const qs = [...v.questions]; [qs[i], qs[i + d]] = [qs[i + d], qs[i]]; set({ questions: qs }) }
  const filter = v.filter ?? {}
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <PageHeader title={id ? 'ویرایش نظرسنجی' : 'نظرسنجی جدید'} />
      <Card className="space-y-4 p-4">
        <Field label="عنوان"><input className="input" value={v.title} onChange={(e) => set({ title: e.target.value })} /></Field>
        <Field label="توضیح برای مشتری"><textarea className="input" rows={2} value={v.description} onChange={(e) => set({ description: e.target.value })} /></Field>
        <Field label="چه زمانی ارسال شود؟"><select className="input" value={v.trigger} disabled={!!id} onChange={(e) => set({ trigger: e.target.value })}>{Object.entries(triggerLabel).map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select></Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="الگوی پیامک دعوت"><select className="input" value={v.sms_template} onChange={(e) => set({ sms_template: e.target.value })}>{sms.data?.map((t) => <option key={t.key} value={t.key}>{t.title}{t.body_id ? '' : ' (تنظیم نشده)'}</option>)}</select></Field>
          <Field label="پیامک پس از ثبت نظر (مثلاً کد تخفیف)"><select className="input" value={v.offer_template} onChange={(e) => set({ offer_template: e.target.value })}><option value="">ندارد</option>{sms.data?.map((t) => <option key={t.key} value={t.key}>{t.title}</option>)}</select></Field>
        </div>
        {v.trigger === 'manual' && (
          <div className="rounded-xl bg-muted p-3"><div className="mb-2 text-sm font-semibold">مخاطبان (از میان دارندگان گارانتی فعال)</div>
            <div className="grid gap-3 sm:grid-cols-2"><Field label="شهر"><input className="input" value={filter.city ?? ''} onChange={(e) => set({ filter: { ...filter, city: e.target.value } })} /></Field><Field label="استان"><input className="input" value={filter.province ?? ''} onChange={(e) => set({ filter: { ...filter, province: e.target.value } })} /></Field></div></div>
        )}
      </Card>
      <h2 className="font-bold">سوال‌ها</h2>
      {v.questions.map((q: any, i: number) => (
        <Card key={i} className="space-y-3 p-4">
          <div className="flex items-center gap-2"><span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-brand-tint text-sm font-bold text-brand num">{fa(i + 1)}</span>
            <input className="input" value={q.text} onChange={(e) => setQ(i, { text: e.target.value })} aria-label="متن سوال" />
            <button className="p-2 text-soft disabled:opacity-30" disabled={i === 0} onClick={() => move(i, -1)} aria-label="بالا"><ArrowUp className="h-4 w-4" /></button>
            <button className="p-2 text-soft disabled:opacity-30" disabled={i === v.questions.length - 1} onClick={() => move(i, 1)} aria-label="پایین"><ArrowDown className="h-4 w-4" /></button>
            <button className="p-2 text-bad" onClick={() => set({ questions: v.questions.filter((_: any, j: number) => j !== i) })} aria-label="حذف"><Trash2 className="h-4 w-4" /></button></div>
          <div className="flex flex-wrap items-center gap-3">
            <select className="input w-auto" value={q.kind} onChange={(e) => setQ(i, { kind: e.target.value })}>{Object.entries(kindLabel).map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select>
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={q.required} onChange={(e) => setQ(i, { required: e.target.checked })} />الزامی</label>
          </div>
          {q.kind === 'choice' && <textarea className="input" rows={3} placeholder="هر گزینه در یک خط" value={(q.options ?? []).join('\n')} onChange={(e) => setQ(i, { options: e.target.value.split('\n').filter(Boolean) })} />}
        </Card>
      ))}
      <button className="btn-ghost w-full" onClick={() => set({ questions: [...v.questions, { text: '', kind: 'rating', options: [], required: false }] })}><Plus className="h-4 w-4" />افزودن سوال</button>
      <ActionButton className="btn-primary w-full" onClick={async () => {
        const r = id ? await put(`/admin/surveys/${id}`, v) : await post('/admin/surveys', v)
        inv('/admin/surveys'); nav(`/admin/surveys/${r.id}`)
      }}>ذخیره</ActionButton>
    </div>
  )
}

export function SurveyResults() {
  const { id } = useParams()
  const s = useGet<any>(`/admin/surveys/${id}`)
  const r = useGet<any>(`/admin/surveys/${id}/results`)
  const can = useCan()
  const inv = useInvalidate()
  const toast = useToast()
  const nav = useNavigate()
  if (s.isLoading || r.isLoading) return <Loading />
  if (s.error) return <ErrorBox error={s.error} />
  const x = r.data
  return (
    <div className="space-y-5">
      <PageHeader title={s.data.title} subtitle={triggerLabel[s.data.trigger]} actions={<>
        <StatusBadge map={surveyStatus} value={s.data.status} />
        {can('surveys.manage') && <>
          <Link to={`/admin/surveys/${id}/edit`} className="btn-ghost btn-sm">ویرایش</Link>
          {s.data.trigger === 'manual' && s.data.status !== 'closed' && <ActionButton className="btn-primary btn-sm" confirm="پیامک دعوت برای مخاطبان جدید ارسال شود؟" onClick={async () => { const q = await post(`/admin/surveys/${id}/launch`); inv('/admin/surveys'); toast.ok(`${fa(q.queued)} دعوت در صف ارسال قرار گرفت`) }}>ارسال دعوت‌ها</ActionButton>}
          {s.data.status !== 'closed' && <ActionButton className="btn-ghost btn-sm" onClick={async () => { await post(`/admin/surveys/${id}/close`); inv('/admin/surveys') }}>بستن</ActionButton>}
          <ActionButton className="btn-ghost btn-sm text-bad" confirm="نظرسنجی حذف شود؟" onClick={async () => { await del(`/admin/surveys/${id}`); inv('/admin/surveys'); nav('/admin/surveys') }}>حذف</ActionButton></>}
      </>} />
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4"><Stat label="دعوت‌شده" value={fa(x.recipients)} /><Stat label="لینک را باز کردند" value={fa(x.opened)} /><Stat label="پاسخ دادند" value={fa(x.responded)} /><Stat label="نرخ پاسخ" value={fa(x.response_rate) + '٪'} tone="ok" /></div>
      {x.questions.map((q: any) => (
        <Card key={q.id} className="p-4">
          <div className="mb-3 flex items-start justify-between gap-3"><b>{q.text}</b><span className="text-xs text-soft">{fa(q.answers)} پاسخ</span></div>
          {(q.kind === 'rating' || q.kind === 'nps') && q.answers > 0 && (
            <div className="space-y-3">
              <div className="flex items-baseline gap-4"><span className="flex items-center gap-1 text-3xl font-extrabold num">{q.kind === 'rating' && <Star className="h-6 w-6 fill-warn text-warn" />}{fa(q.average)}</span>{q.kind === 'nps' && <Badge tone={q.nps >= 30 ? 'ok' : q.nps >= 0 ? 'warn' : 'bad'}>NPS: {fa(q.nps)}</Badge>}</div>
              <Bars dist={q.distribution} keys={q.kind === 'rating' ? ['5', '4', '3', '2', '1'] : Array.from({ length: 11 }, (_, i) => String(10 - i))} total={q.answers} />
            </div>
          )}
          {q.kind === 'yes_no' && q.answers > 0 && <Bars dist={q.distribution} keys={['1', '0']} labels={{ '1': 'بله', '0': 'خیر' }} total={q.answers} />}
          {q.kind === 'choice' && q.answers > 0 && <Bars dist={q.choices} keys={Object.keys(q.choices)} total={q.answers} />}
          {q.kind === 'text' && <div className="space-y-2">{q.texts?.length ? q.texts.map((t: any, i: number) => <p key={i} className="rounded-xl bg-muted p-3 text-sm">{t.answer_text}</p>) : <span className="text-sm text-soft">پاسخی نیست</span>}</div>}
          {q.answers === 0 && q.kind !== 'text' && <span className="text-sm text-soft">هنوز پاسخی ثبت نشده</span>}
        </Card>
      ))}
      {!!x.technicians.length && (
        <Card><div className="border-b border-line p-4 font-bold">رضایت مشتریان از هر تکنسین</div>
          {x.technicians.map((t: any) => <div key={t.technician_id} className="flex items-center justify-between border-b border-line/60 p-4 text-sm last:border-0"><span>{t.technician_name}</span><span className="flex items-center gap-3"><span className="text-soft">{fa(t.responses)} نظر</span><b className="flex items-center gap-1 num"><Star className="h-4 w-4 fill-warn text-warn" />{fa(t.average)}</b></span></div>)}</Card>
      )}
    </div>
  )
}

function Bars({ dist, keys, total, labels }: { dist: Record<string, number>; keys: string[]; total: number; labels?: Record<string, string> }) {
  return (
    <div className="space-y-1.5">
      {keys.map((k) => {
        const n = dist?.[k] ?? 0
        return (
          <div key={k} className="flex items-center gap-2 text-xs"><span className="w-20 shrink-0 truncate text-soft">{labels?.[k] ?? fa(k)}</span>
            <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-brand" style={{ width: `${(n * 100) / total}%` }} /></div><span className="w-8 text-end num">{fa(n)}</span></div>
        )
      })}
    </div>
  )
}
