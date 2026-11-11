import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, ArrowRight, Copy, Download, Hash, Plus, Trash2, Type } from 'lucide-react'
import clsx from 'clsx'
import { ActionButton, Field, JalaliInput, Modal, NumInput, useToast } from './ui'
import { post } from '../lib/api'
import { dateLong, fa, todayISO } from '../lib/format'

export interface Token { type: 'text' | 'number' | 'year' | 'month' | 'day' | 'serial'; value?: string; digits?: number; calendar?: 'jalali' | 'gregorian' }

const KINDS: { type: Token['type']; label: string; hint: string; make: () => Token }[] = [
  { type: 'text', label: 'متن ثابت', hint: 'پیشوند، پسوند یا جداکننده (مثل FZ یا -)', make: () => ({ type: 'text', value: '' }) },
  { type: 'year', label: 'سال', hint: 'سال تولید', make: () => ({ type: 'year', digits: 4, calendar: 'jalali' }) },
  { type: 'month', label: 'ماه', hint: 'ماه تولید (دو رقم)', make: () => ({ type: 'month' }) },
  { type: 'day', label: 'روز', hint: 'روز تولید (دو رقم)', make: () => ({ type: 'day' }) },
  { type: 'number', label: 'عدد ثابت', hint: 'رقم‌های ثابت (مثل کد مدل 240010)', make: () => ({ type: 'number', value: '' }) },
  { type: 'serial', label: 'سریال', hint: 'شماره ردیف دستگاه', make: () => ({ type: 'serial', digits: 5 }) },
]
const TEMPLATES: { label: string; formula: Token[] }[] = [
  { label: 'اوینکی ۱۴۰۵: سال + FZ + ماه + سریال ۵ رقمی', formula: [{ type: 'year', digits: 4, calendar: 'jalali' }, { type: 'text', value: 'FZ' }, { type: 'month' }, { type: 'serial', digits: 5 }] },
  { label: 'پیشوند - سال ۲ رقمی ماه روز - سریال', formula: [{ type: 'text', value: 'EV-' }, { type: 'year', digits: 2, calendar: 'jalali' }, { type: 'month' }, { type: 'day' }, { type: 'text', value: '-' }, { type: 'serial', digits: 4 }] },
  { label: 'فقط سال میلادی + سریال', formula: [{ type: 'year', digits: 4, calendar: 'gregorian' }, { type: 'serial', digits: 6 }] },
]
const chipTone: Record<string, string> = { text: 'border-sky-400/50 bg-sky-500/10', number: 'border-violet-400/50 bg-violet-500/10', year: 'border-emerald-400/50 bg-emerald-500/10', month: 'border-emerald-400/50 bg-emerald-500/10', day: 'border-emerald-400/50 bg-emerald-500/10', serial: 'border-brand/50 bg-brand-tint' }
const label = (t: Token['type']) => KINDS.find((k) => k.type === t)!.label

/**
 * Builds a guarantee-code formula from pieces. The page is right-to-left but a CODE is always read left-to-right,
 * so the whole formula row and the preview are forced to dir="ltr" (and codes are wrapped in <bdi>): the first piece
 * is the leftmost character of the code, exactly as it will be printed on the card.
 */
export function CodeFormulaBuilder({ value, onChange }: { value: Token[] | null | undefined; onChange: (f: Token[]) => void }) {
  const f: Token[] = Array.isArray(value) ? value : []
  const [date, setDate] = useState(todayISO())
  const [serial, setSerial] = useState('1')
  const [prev, setPrev] = useState<any>(null)

  const key = useMemo(() => JSON.stringify([f, date, serial]), [f, date, serial])
  useEffect(() => {
    if (!f.length) { setPrev(null); return }
    const t = setTimeout(async () => { try { setPrev(await post('/admin/code-formula/preview', { formula: f, date, serial: +serial || 1 })) } catch { setPrev({ valid: false, error: 'پیش‌نمایش در دسترس نیست' }) } }, 250)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  const set = (i: number, p: Partial<Token>) => onChange(f.map((t, j) => (j === i ? { ...t, ...p } : t)))
  const move = (i: number, d: number) => { const n = [...f]; const j = i + d; if (j < 0 || j >= n.length) return; [n[i], n[j]] = [n[j], n[i]]; onChange(n) }
  const hasDate = f.some((t) => t.type === 'year')

  return (
    <div className="space-y-3 rounded-2xl border border-line bg-muted/40 p-3">
      <div className="flex flex-wrap gap-1.5">
        {KINDS.map((k) => <button key={k.type} type="button" title={k.hint} className="btn-ghost btn-sm" onClick={() => onChange([...f, k.make()])}><Plus className="h-3.5 w-3.5" />{k.label}</button>)}
        <select className="input ms-auto w-auto min-h-[36px] py-0 text-xs" value="" onChange={(e) => { if (e.target.value) onChange(TEMPLATES[+e.target.value].formula) }}>
          <option value="">نمونه‌های آماده…</option>{TEMPLATES.map((t, i) => <option key={i} value={i}>{t.label}</option>)}</select>
      </div>

      {/* the code reads left to right */}
      <div dir="ltr" className="flex min-h-[64px] flex-wrap items-stretch gap-2 rounded-xl border border-dashed border-line bg-surface p-2">
        {!f.length && <span dir="rtl" className="m-auto text-sm text-soft">با دکمه‌های بالا بخش‌های کد را اضافه کنید؛ ترتیب از چپ به راست همان ترتیب نوشته‌شدن کد است.</span>}
        {f.map((t, i) => (
          <div key={i} className={clsx('flex flex-col gap-1.5 rounded-xl border p-2', chipTone[t.type])}>
            <div className="flex items-center justify-between gap-2 text-[11px] font-semibold text-soft" dir="rtl">
              <span>{label(t.type)}</span>
              <span className="flex items-center">
                <button type="button" className="rounded p-1 hover:bg-black/5" onClick={() => move(i, -1)} aria-label="انتقال به چپ"><ArrowLeft className="h-3.5 w-3.5" /></button>
                <button type="button" className="rounded p-1 hover:bg-black/5" onClick={() => move(i, 1)} aria-label="انتقال به راست"><ArrowRight className="h-3.5 w-3.5" /></button>
                <button type="button" className="rounded p-1 text-bad hover:bg-bad/10" onClick={() => onChange(f.filter((_, j) => j !== i))} aria-label="حذف"><Trash2 className="h-3.5 w-3.5" /></button>
              </span>
            </div>
            {t.type === 'text' && <input className="input num w-28 min-h-[36px] uppercase" dir="ltr" placeholder="FZ" value={t.value ?? ''} onChange={(e) => set(i, { value: e.target.value.replace(/\s/g, '') })} />}
            {t.type === 'number' && <input className="input num w-28 min-h-[36px]" dir="ltr" inputMode="numeric" placeholder="240010" value={t.value ?? ''} onChange={(e) => set(i, { value: e.target.value.replace(/\D/g, '') })} />}
            {t.type === 'year' && (
              <div className="flex gap-1.5" dir="rtl">
                <select className="input min-h-[36px] w-auto py-0 text-xs" value={t.digits} onChange={(e) => set(i, { digits: +e.target.value })}><option value={4}>۴ رقمی</option><option value={2}>۲ رقمی</option></select>
                <select className="input min-h-[36px] w-auto py-0 text-xs" value={t.calendar ?? 'jalali'} onChange={(e) => set(i, { calendar: e.target.value as any })}><option value="jalali">شمسی</option><option value="gregorian">میلادی</option></select>
              </div>)}
            {(t.type === 'month' || t.type === 'day') && <div className="grid min-h-[36px] place-items-center px-2 text-xs text-soft" dir="rtl">دو رقمی ({t.type === 'month' ? '۰۱–۱۲' : '۰۱–۳۱'})</div>}
            {t.type === 'serial' && (
              <div className="flex items-center gap-1.5" dir="rtl"><span className="text-xs text-soft">تعداد رقم</span>
                <input className="input num min-h-[36px] w-16 px-2 text-center" dir="ltr" inputMode="numeric" value={t.digits ?? ''} onChange={(e) => set(i, { digits: Math.min(12, +e.target.value.replace(/\D/g, '') || 0) })} /></div>)}
          </div>))}
      </div>

      {f.length > 0 && (
        <div className="grid gap-3 rounded-xl bg-surface p-3 sm:grid-cols-[1fr_auto]">
          <div>
            <div className="text-xs text-soft">نمونهٔ کد ساخته‌شده</div>
            {prev?.valid ? <bdi dir="ltr" className="mt-1 block break-all font-mono text-xl font-bold tracking-wider text-brand">{prev.example}</bdi>
              : <div className="mt-1 text-sm text-bad">{prev?.error ?? '…'}</div>}
            {prev?.valid && <div className="mt-1 text-xs text-soft">{hasDate ? (prev.manufacture_date ? <>تاریخ تولید خوانده‌شده از کد: {dateLong(prev.manufacture_date)}{prev.date_approx ? ' (آخرین روز ماه/سال، بهترین حالت برای مشتری)' : ''}</> : null) : 'این فرمول تاریخ ندارد؛ مدت گارانتی از تاریخ خرید حساب می‌شود.'}</div>}
            {prev?.valid && <details className="mt-1 text-xs text-soft"><summary className="cursor-pointer">الگوی اعتبارسنجی (Regex)</summary><bdi dir="ltr" className="mt-1 block break-all font-mono">{prev.regex}</bdi></details>}
          </div>
          <div className="flex items-end gap-2" dir="rtl">
            <Field label="تاریخ نمونه"><JalaliInput value={date} onChange={(v) => v && setDate(v)} clearable={false} /></Field>
            <Field label="سریال"><NumInput className="w-24" value={serial} onChange={setSerial} /></Field>
          </div>
        </div>)}
      <p className="flex items-start gap-1.5 text-xs leading-6 text-soft"><Hash className="mt-1 h-3.5 w-3.5 shrink-0" /><span>کدی که مشتری وارد می‌کند باید دقیقاً با این فرمول بخواند (حروف کوچک و بزرگ فرقی ندارد). اگر فرمول سال و ماه داشته باشد، «تاریخ تولید» از خود کد خوانده می‌شود و گارانتی از آن تاریخ شروع می‌شود؛ اگر فقط ماه باشد آخرین روز آن ماه (و اگر فقط سال باشد پایان سال) در نظر گرفته می‌شود.</span></p>
    </div>
  )
}

/** Prints a batch of codes from the product's saved formula (for guarantee cards / the production line). */
export function CodeGenerator({ product, onClose }: { product: { id: number; name: string } | null; onClose: () => void }) {
  const toast = useToast()
  const [f, setF] = useState({ date: todayISO(), start: '1', count: '100' })
  const [codes, setCodes] = useState<string[]>([])
  if (!product) return null
  const csv = () => {
    const url = URL.createObjectURL(new Blob(['﻿' + codes.join('\r\n')], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a'); a.href = url; a.download = `codes-${product.id}.csv`; a.click(); URL.revokeObjectURL(url)
  }
  return (
    <Modal open wide onClose={onClose} title={`تولید کد گارانتی — ${product.name}`}>
      <div className="grid gap-3 sm:grid-cols-4">
        <Field label="تاریخ تولید"><JalaliInput value={f.date} onChange={(v) => v && setF({ ...f, date: v })} clearable={false} /></Field>
        <Field label="شروع سریال"><NumInput value={f.start} onChange={(v) => setF({ ...f, start: v })} /></Field>
        <Field label="تعداد (حداکثر ۵۰۰۰)"><NumInput value={f.count} onChange={(v) => setF({ ...f, count: v })} /></Field>
        <div className="flex items-end"><ActionButton onClick={async () => { const r = await post(`/admin/products/${product.id}/codes/generate`, { date: f.date, start: +f.start || 1, count: +f.count || 1 }); setCodes(r.codes) }}><Type className="h-4 w-4" />تولید</ActionButton></div>
      </div>
      {codes.length > 0 && (
        <div className="mt-4">
          <div className="mb-2 flex items-center justify-between text-sm"><b>{fa(codes.length)} کد</b>
            <span className="flex gap-2"><button className="btn-ghost btn-sm" onClick={() => { navigator.clipboard?.writeText(codes.join('\n')); toast.ok('کپی شد') }}><Copy className="h-4 w-4" />کپی</button>
              <button className="btn-ghost btn-sm" onClick={csv}><Download className="h-4 w-4" />فایل اکسل</button></span></div>
          <div dir="ltr" className="max-h-64 overflow-auto rounded-xl border border-line bg-muted p-3 font-mono text-sm leading-7">{codes.slice(0, 500).map((c) => <div key={c}><bdi>{c}</bdi></div>)}{codes.length > 500 && <div className="text-soft">… و {fa(codes.length - 500)} کد دیگر در فایل</div>}</div>
        </div>)}
    </Modal>
  )
}
