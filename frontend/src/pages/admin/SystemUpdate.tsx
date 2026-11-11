import { useRef, useState } from 'react'
import { AlertTriangle, CheckCircle2, Loader2, PackageOpen, ServerCog, Upload, XCircle } from 'lucide-react'
import clsx from 'clsx'
import { Badge, Card, ErrorBox, Loading, PageHeader, useToast } from '../../components/ui'
import { getSession } from '../../lib/api'
import { dateTime, fa } from '../../lib/format'
import { useGet, useInvalidate } from '../../lib/hooks'

const STEPS: [string, string][] = [['queued', 'در صف'], ['backing_up', 'پشتیبان‌گیری'], ['loading', 'بارگذاری ایمیج‌ها'], ['restarting', 'راه‌اندازی مجدد'], ['verifying', 'بررسی سلامت'], ['done', 'پایان']]
const mb = (n: number) => fa(Math.round((n / 1048576) * 10) / 10)

/**
 * Update the running system with the image bundle built on a development computer (`make images-bundle`).
 * The file is streamed to the server; a small service on the server then backs up, loads the images, restarts and
 * rolls back by itself if the new version does not become healthy.
 */
export function SystemUpdate() {
  const toast = useToast()
  const inv = useInvalidate()
  const [busyUp, setBusyUp] = useState(false)
  const [prog, setProg] = useState<{ done: number; total: number; speed: number } | null>(null)
  const [drag, setDrag] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const xhr = useRef<XMLHttpRequest | null>(null)
  const s = useGet<any>('/admin/system/update', { refetchInterval: 3000 })

  const upload = (file: File) => {
    if (!/\.(tar\.gz|tgz)$/i.test(file.name)) { toast.info('فایل باید بستهٔ .tar.gz ساخته‌شده با make images-bundle باشد'); return }
    if (!window.confirm(`سیستم با «${file.name}» (${mb(file.size)} مگابایت) به‌روزرسانی شود؟\nقبل از شروع، از پایگاه داده پشتیبان گرفته می‌شود و در صورت خطا نسخهٔ قبلی برمی‌گردد. در لحظهٔ راه‌اندازی مجدد، چند ثانیه سایت در دسترس نیست.`)) return
    const x = new XMLHttpRequest()
    xhr.current = x
    const t0 = Date.now()
    x.open('PUT', '/api/v1/admin/system/update')
    x.setRequestHeader('Authorization', 'Bearer ' + (getSession()?.access_token ?? ''))
    x.setRequestHeader('X-Filename', file.name)
    x.setRequestHeader('Content-Type', 'application/octet-stream')
    x.upload.onprogress = (e) => setProg({ done: e.loaded, total: e.total, speed: e.loaded / Math.max(1, (Date.now() - t0) / 1000) })
    x.onload = () => {
      setBusyUp(false); setProg(null)
      if (x.status === 202) { toast.ok('فایل بارگذاری شد؛ به‌روزرسانی شروع می‌شود'); inv('/admin/system/update'); return }
      let msg = x.status === 413 ? 'فایل از حد مجاز سرور بزرگ‌تر است (nginx سرور را با make ssl دوباره تنظیم کنید)' : 'بارگذاری ناموفق بود'
      try { const j = JSON.parse(x.responseText); msg = j?.error?.message ?? j?.message ?? msg } catch { /* keep the default */ }
      toast.error(msg)
    }
    x.onerror = () => { setBusyUp(false); setProg(null); toast.error('ارتباط با سرور قطع شد') }
    x.onabort = () => { setBusyUp(false); setProg(null) }
    setBusyUp(true); setProg({ done: 0, total: file.size, speed: 0 })
    x.send(file)
  }

  if (s.isLoading) return <Loading />
  if (s.error) return <ErrorBox error={s.error} retry={() => s.refetch()} />
  const d = s.data
  const st = d.status ?? {}
  const idx = STEPS.findIndex(([k]) => k === st.state)
  const failed = st.state === 'failed' || st.state === 'rolling_back'

  return (
    <div className="max-w-3xl space-y-5">
      <PageHeader title="به‌روزرسانی سیستم" subtitle="نسخهٔ جدید را روی کامپیوتر توسعه بسازید (make images-bundle) و فایل فشرده را همین‌جا بارگذاری کنید." />

      <Card className={clsx('flex items-start gap-3 p-4', d.updater_online ? '' : 'border-warn/50 bg-warn/10')}>
        <ServerCog className={clsx('mt-0.5 h-6 w-6 shrink-0', d.updater_online ? 'text-ok' : 'text-warn')} />
        <div className="text-sm leading-7">
          {d.updater_online
            ? <><b>سرویس به‌روزرسانی روی سرور فعال است</b><div className="text-soft">آخرین پاسخ: {dateTime(d.updater?.seen_at)} · معماری سرور: <span dir="ltr">{d.updater?.arch}</span> (بسته را برای همین معماری بسازید)</div></>
            : <><b>سرویس به‌روزرسانی روی سرور نصب یا فعال نیست.</b><div>یک‌بار روی سرور، داخل پوشهٔ پروژه اجرا کنید:</div><pre dir="ltr" className="mt-1 rounded-lg bg-surface p-2 text-xs">sudo make install-updater</pre></>}
        </div>
      </Card>

      <Card className="p-5">
        <div onDragOver={(e) => { e.preventDefault(); setDrag(true) }} onDragLeave={() => setDrag(false)}
          onDrop={(e) => { e.preventDefault(); setDrag(false); const f = e.dataTransfer.files?.[0]; if (f && !busyUp && !d.busy) upload(f) }}
          className={clsx('grid place-items-center rounded-2xl border-2 border-dashed p-8 text-center transition', drag ? 'border-brand bg-brand-tint' : 'border-line')}>
          {busyUp && prog ? (
            <div className="w-full max-w-md">
              <div className="mb-2 flex items-center justify-between text-sm"><span className="flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />در حال بارگذاری…</span><b className="num">{fa(Math.round((prog.done / prog.total) * 100))}٪</b></div>
              <div className="h-3 overflow-hidden rounded-full bg-muted"><div className="h-full bg-brand transition-all" style={{ width: `${(prog.done / prog.total) * 100}%` }} /></div>
              <div className="mt-2 text-xs text-soft num">{mb(prog.done)} از {mb(prog.total)} مگابایت · {mb(prog.speed)} مگابایت بر ثانیه</div>
              <button className="btn-ghost btn-sm mt-3" onClick={() => xhr.current?.abort()}>انصراف</button>
            </div>
          ) : (
            <>
              <PackageOpen className="mb-2 h-10 w-10 text-soft" />
              <p className="font-semibold">فایل ایمیج‌ها را اینجا رها کنید</p>
              <p className="mb-3 text-xs text-soft">evinki-images.tar.gz — خروجی دستور make images-bundle</p>
              <button className="btn-primary" disabled={d.busy || !d.updater_online} onClick={() => input.current?.click()}><Upload className="h-4 w-4" />انتخاب فایل و شروع</button>
              {!d.updater_online && <p className="mt-2 text-xs text-warn">ابتدا سرویس به‌روزرسانی را روی سرور نصب کنید.</p>}
              <input ref={input} type="file" accept=".gz,.tgz,application/gzip" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) upload(f) }} />
            </>)}
        </div>
      </Card>

      {st.state && (
        <Card className="p-5">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 className="font-bold">وضعیت آخرین به‌روزرسانی</h3>
            {st.state === 'done' ? <Badge tone="ok">موفق</Badge> : failed ? <Badge tone="bad">ناموفق — نسخهٔ قبلی برگردانده شد</Badge> : <Badge tone="warn">در حال اجرا</Badge>}
          </div>
          {d.request && <p className="mb-3 text-xs text-soft">فایل: <span dir="ltr">{d.request.name}</span> · {mb(d.request.size)} مگابایت · بارگذاری‌کننده: {d.request.uploaded_by} · {dateTime(d.request.uploaded_at)}</p>}
          <ol className="mb-3 grid gap-2 sm:grid-cols-6">
            {STEPS.map(([k, label], i) => {
              const state = failed ? (i < Math.max(idx, 1) ? 'ok' : 'todo') : i < idx || st.state === 'done' ? 'ok' : i === idx ? 'now' : 'todo'
              return <li key={k} className={clsx('flex items-center gap-1.5 rounded-xl border p-2 text-xs', state === 'ok' ? 'border-ok/40 bg-ok/10 text-ok' : state === 'now' ? 'border-warn/50 bg-warn/10' : 'border-line text-soft')}>
                {state === 'ok' ? <CheckCircle2 className="h-4 w-4" /> : state === 'now' ? <Loader2 className="h-4 w-4 animate-spin" /> : <span className="h-4 w-4 rounded-full border border-line" />}{label}</li>
            })}
          </ol>
          <div className={clsx('flex items-start gap-2 rounded-xl p-3 text-sm', failed ? 'bg-bad/10 text-bad' : st.state === 'done' ? 'bg-ok/10 text-ok' : 'bg-muted')}>
            {failed ? <XCircle className="mt-0.5 h-4 w-4 shrink-0" /> : d.busy ? <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin" /> : <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />}{st.message}
          </div>
          {d.log && <details className="mt-3"><summary className="cursor-pointer text-sm text-soft">گزارش فنی (log)</summary><pre dir="ltr" className="mt-2 max-h-64 overflow-auto rounded-xl bg-muted p-3 text-xs leading-5">{d.log}</pre></details>}
          {st.state === 'done' && <button className="btn-primary mt-3" onClick={() => window.location.reload()}>بارگذاری مجدد صفحه برای دیدن نسخهٔ جدید</button>}
        </Card>)}

      {!!d.history?.length && (
        <Card className="p-5"><h3 className="mb-2 font-bold">تاریخچه</h3>
          <ul className="divide-y divide-line/60 text-sm">{d.history.map((h: any) => <li key={h.id} className="flex flex-wrap items-center justify-between gap-2 py-2"><span dir="ltr" className="text-xs text-soft">{h.file?.split('/').pop()}</span><span>{h.message}</span><span className="flex items-center gap-2 text-xs text-soft">{dateTime(h.finished_at)}{h.ok ? <Badge tone="ok">موفق</Badge> : <Badge tone="bad">ناموفق</Badge>}</span></li>)}</ul></Card>)}

      <p className="flex items-start gap-2 text-xs leading-6 text-soft"><AlertTriangle className="mt-1 h-3.5 w-3.5 shrink-0" />پیش از هر به‌روزرسانی خودکار از پایگاه داده و فایل‌ها پشتیبان گرفته می‌شود (پوشهٔ backups). تغییرات پایگاه داده فقط افزایشی‌اند؛ اگر نسخهٔ جدید راه نیفتد، ایمیج‌های قبلی برمی‌گردند.</p>
    </div>
  )
}
