import { useCallback, useEffect, useRef, useState } from 'react'
import { CheckCircle2, Delete, KeyRound, Loader2, LogIn, LogOut, RefreshCw, ScanFace, ShieldAlert, Users, Wifi, WifiOff, XCircle } from 'lucide-react'
import clsx from 'clsx'
import { fa } from '../../lib/format'
import { faceEngine, findBestMatch, DEFAULT_AMBIGUITY_MARGIN, DEFAULT_THRESHOLD, FACE_MODEL_NAME, similarityToConfidence, type MatchCandidate } from '../../lib/face/faceEngine'
import { enqueue, gallerySavedAt, lastKindFor, loadGallery, newUuid, queued, queueSize, rememberLastKind, removeMany, saveGallery } from '../../lib/face/offlineQueue'

/**
 * The entrance tablet: full-screen, face recognition in the browser (works offline), personnel-code + PIN fallback,
 * punches queued locally when the network is down and sent later. It signs in with the device key, not a user account.
 */

const KEY = 'evinki.kiosk.key'
const CFG = 'evinki.kiosk.cfg'
const APP_VERSION = 'web-1.0'
const HOLD_MS = 1800            // keep the face in front of the camera this long (anti-photo, anti-accident)
const GREETING_MS = 3500
const SYNC_MS = 2 * 60_000

interface Cfg { device: string; face: boolean; pin: boolean; liveness: boolean; threshold: number; ambiguity_margin: number; cooldown_seconds: number; company: string }
const DEFAULT_CFG: Cfg = { device: '', face: true, pin: true, liveness: true, threshold: DEFAULT_THRESHOLD, ambiguity_margin: DEFAULT_AMBIGUITY_MARGIN, cooldown_seconds: 60, company: 'اوینکی' }

const keyStore = { get: () => { try { return localStorage.getItem(KEY) ?? '' } catch { return '' } }, set: (v: string) => { try { v ? localStorage.setItem(KEY, v) : localStorage.removeItem(KEY) } catch { /* private mode */ } } }

async function kapi<T = any>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch('/api/v1/kiosk' + path, { method, headers: { 'Content-Type': 'application/json', 'X-Device-Key': keyStore.get() }, body: body === undefined ? undefined : JSON.stringify(body) })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) throw Object.assign(new Error(data?.error?.message ?? data?.message ?? 'خطا'), { status: res.status })
  return data
}

const digits = (s: string) => s.replace(/\d/g, (d) => '۰۱۲۳۴۵۶۷۸۹'[+d])
const tehran = (o: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat('fa-IR-u-ca-persian', { timeZone: 'Asia/Tehran', ...o })
const timeFmt = tehran({ hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
const dateFmt = tehran({ weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })

function useClock() {
  const [now, setNow] = useState(new Date())
  useEffect(() => { const t = setInterval(() => setNow(new Date()), 1000); return () => clearInterval(t) }, [])
  return now
}
function useOnline() {
  const [on, setOn] = useState(navigator.onLine)
  useEffect(() => { const u = () => setOn(true), d = () => setOn(false); addEventListener('online', u); addEventListener('offline', d); return () => { removeEventListener('online', u); removeEventListener('offline', d) } }, [])
  return on
}

interface Greeting { ok: boolean; name: string; kind: 'in' | 'out'; message: string; offline?: boolean }

export default function Kiosk() {
  const [key, setKey] = useState(keyStore.get())
  const [cfg, setCfg] = useState<Cfg>(() => { try { return { ...DEFAULT_CFG, ...JSON.parse(localStorage.getItem(CFG) || '{}') } } catch { return DEFAULT_CFG } })
  const [screen, setScreen] = useState<'scan' | 'pin'>('scan')
  const [greet, setGreet] = useState<Greeting | null>(null)
  const [hint, setHint] = useState('')
  const [progress, setProgress] = useState(0)
  const [pending, setPending] = useState(0)
  const [candidates, setCandidates] = useState<MatchCandidate[]>([])
  const [galleryAt, setGalleryAt] = useState<number | null>(null)
  const [ready, setReady] = useState(faceEngine.ready)
  const [camErr, setCamErr] = useState('')
  const [authErr, setAuthErr] = useState('')
  const online = useOnline()
  const now = useClock()
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const busy = useRef(false)
  const hold = useRef<{ id: number; since: number } | null>(null)
  const lastPunch = useRef<Record<number, number>>({})
  const cfgRef = useRef(cfg); cfgRef.current = cfg
  const candRef = useRef(candidates); candRef.current = candidates
  const screenRef = useRef(screen); screenRef.current = screen
  const greetRef = useRef(greet); greetRef.current = greet

  // ── server config + gallery ──
  const loadConfig = useCallback(async () => {
    if (!key) return
    try {
      const c = await kapi('GET', '/config')
      const next: Cfg = { device: c.device, face: !!c.face, pin: !!c.pin, liveness: !!c.liveness, threshold: Number(c.threshold) || DEFAULT_THRESHOLD, ambiguity_margin: Number(c.ambiguity_margin) || DEFAULT_AMBIGUITY_MARGIN, cooldown_seconds: Number(c.cooldown_seconds) || 0, company: c.company || 'اوینکی' }
      setCfg(next); setAuthErr('')
      try { localStorage.setItem(CFG, JSON.stringify(next)) } catch { /* ignore */ }
    } catch (e: any) { if (e?.status === 401) setAuthErr('کلید دستگاه نامعتبر است یا دستگاه غیرفعال شده.') }
  }, [key])

  const applyGallery = useCallback((g: { items: any[] }) => {
    setCandidates(g.items.map((i) => ({ employeeId: i.employee_id, fullName: i.name, personnelCode: i.personnel_code, vectors: (i.vectors as number[][]).map((v) => Float32Array.from(v)) })))
  }, [])
  const refreshGallery = useCallback(async () => {
    const cached = await loadGallery()
    if (cached && !candRef.current.length) { applyGallery(cached); setGalleryAt(await gallerySavedAt()) }
    try {
      const v = (await kapi('GET', '/gallery/version')).version as string
      if (!cached || cached.version !== v) {
        const items = await kapi<any[]>('GET', '/gallery')
        const g = { version: v, items }
        await saveGallery(g as any); applyGallery(g); setGalleryAt(Date.now())
      }
    } catch { /* offline: keep the cached gallery */ }
  }, [applyGallery])

  useEffect(() => { if (!key) return; void loadConfig(); void refreshGallery(); const t = setInterval(() => { void loadConfig(); void refreshGallery() }, SYNC_MS); return () => clearInterval(t) }, [key, loadConfig, refreshGallery])
  useEffect(() => { if (key && cfg.face && !faceEngine.ready) faceEngine.load().then(() => setReady(true)).catch(() => setCamErr('بارگذاری مدل تشخیص چهره ناموفق بود')) }, [key, cfg.face])

  // ── offline queue ──
  const flush = useCallback(async () => {
    if (!navigator.onLine || !keyStore.get()) return
    const items = await queued()
    if (items.length) {
      try {
        const res = await kapi('POST', '/punches', { punches: items.map((i) => ({ client_uuid: i.client_uuid, employee_id: i.employee_id, kind: i.kind, method: i.method, happened_at: i.happened_at, confidence: i.confidence ?? null, created_offline: true })) })
        // the server answered: accepted and duplicate punches are stored; rejected ones (unknown employee, bad data) can never succeed
        if (typeof res.accepted === 'number') await removeMany(items.map((i) => i.client_uuid))
      } catch { /* try again later */ }
    }
    setPending(await queueSize())
    try { await kapi('POST', '/heartbeat', { app_version: APP_VERSION, pending_count: await queueSize() }) } catch { /* ignore */ }
  }, [])
  useEffect(() => { void queueSize().then(setPending) }, [])
  useEffect(() => { if (!key || !online) return; void flush(); const t = setInterval(() => void flush(), SYNC_MS); return () => clearInterval(t) }, [key, online, flush])

  // ── camera ──
  useEffect(() => {
    if (!key || !cfg.face || screen !== 'scan') return
    let stream: MediaStream | null = null, cancelled = false
    if (!navigator.mediaDevices?.getUserMedia) { setCamErr('دوربین فقط روی آدرس امن (https) کار می‌کند.'); return }
    navigator.mediaDevices.getUserMedia({ video: { facingMode: 'user', width: { ideal: 1280 }, height: { ideal: 720 } }, audio: false })
      .then(async (s) => { stream = s; if (cancelled) { s.getTracks().forEach((t) => t.stop()); return } if (videoRef.current) { videoRef.current.srcObject = s; await videoRef.current.play(); setCamErr('') } })
      .catch((e) => setCamErr(e?.name === 'NotAllowedError' ? 'دسترسی به دوربین داده نشده؛ از تنظیمات مرورگر فعال کنید.' : 'دوربین در دسترس نیست.'))
    return () => { cancelled = true; stream?.getTracks().forEach((t) => t.stop()) }
  }, [key, cfg.face, screen])

  // ── punching ──
  const punch = useCallback(async (employeeId: number, name: string, method: 'face' | 'pin', confidence: number | null) => {
    const remembered = await lastKindFor(employeeId)
    const kind: 'in' | 'out' = remembered === 'in' ? 'out' : 'in'
    const p = { client_uuid: newUuid(), employee_id: employeeId, kind, method, happened_at: new Date().toISOString(), confidence, created_offline: false }
    if (navigator.onLine) {
      try {
        const r = await kapi('POST', '/punches', { punches: [p] })
        if (r.rejected === 0) { await rememberLastKind(employeeId, kind); return { ok: true, name, kind, message: 'ثبت شد' } as Greeting }
      } catch { /* fall through to the queue */ }
    }
    await enqueue({ client_uuid: p.client_uuid, employee_id: employeeId, employee_name: name, kind, method, happened_at: p.happened_at, confidence, snapshot_base64: null })
    await rememberLastKind(employeeId, kind)
    setPending(await queueSize())
    return { ok: true, name, kind, message: 'ذخیره شد؛ پس از اتصال به اینترنت ارسال می‌شود', offline: true } as Greeting
  }, [])

  const show = useCallback((g: Greeting) => { setGreet(g); setTimeout(() => setGreet((cur) => (cur === g ? null : cur)), GREETING_MS) }, [])

  // ── recognition loop ──
  useEffect(() => {
    if (!key || !cfg.face || !ready || screen !== 'scan') return
    const timer = setInterval(async () => {
      if (busy.current || greetRef.current || screenRef.current !== 'scan') return
      const video = videoRef.current
      if (!video || video.readyState < 2) return
      busy.current = true
      try {
        const { face, count } = await faceEngine.detectPrimary(video)
        if (count === 0 || !face) { hold.current = null; setProgress(0); setHint(''); return }
        if (count > 1) { hold.current = null; setProgress(0); setHint('لطفاً هر بار یک نفر جلوی دوربین بایستد'); return }
        if (face.box.width < video.videoWidth * 0.18) { hold.current = null; setProgress(0); setHint('کمی نزدیک‌تر شوید'); return }
        if (face.score < 0.7) { setHint('رو به دوربین نگاه کنید'); return }
        if (!candRef.current.length) { setHint('هنوز چهره‌ای ثبت نشده است؛ از کد پرسنلی استفاده کنید'); return }
        const emb = await faceEngine.getEmbedding(video, face)
        const c = cfgRef.current
        const m = findBestMatch(emb, candRef.current, c.threshold, c.ambiguity_margin)
        if (!m) { hold.current = null; setProgress(0); setHint('شناسایی نشد — از «ورود با کد پرسنلی» استفاده کنید'); return }
        const id = m.candidate.employeeId
        if (!hold.current || hold.current.id !== id) hold.current = { id, since: Date.now() }
        const need = c.liveness ? HOLD_MS : 0
        const p = need ? Math.min(1, (Date.now() - hold.current.since) / need) : 1
        setProgress(p); setHint(`${m.candidate.fullName} — چند لحظه ثابت بمانید`)
        if (p < 1) return
        hold.current = null; setProgress(0)
        const last = lastPunch.current[id] ?? 0
        if (Date.now() - last < c.cooldown_seconds * 1000) { show({ ok: false, name: m.candidate.fullName, kind: 'in', message: 'چند لحظه پیش ثبت شده است' }); return }
        lastPunch.current[id] = Date.now()
        show(await punch(id, m.candidate.fullName, 'face', similarityToConfidence(m.similarity, c.threshold)))
      } catch { /* a bad frame: skip */ } finally { busy.current = false }
    }, 450)
    return () => clearInterval(timer)
  }, [key, cfg.face, ready, screen, punch, show])

  if (!key || authErr) return <Setup error={authErr} onSave={(k) => { keyStore.set(k); setKey(k); setAuthErr('') }} />

  const faceUnavailable = !cfg.face
  return (
    <div className="fixed inset-0 flex flex-col bg-neutral-950 text-white select-none" dir="rtl">
      <header className="flex items-center justify-between gap-3 px-5 py-3">
        <div className="flex items-center gap-3"><img src="/logo-evinki.png" alt="" className="h-9 rounded bg-white/95 px-2 py-1" /><div><div className="text-sm font-bold">{cfg.company}</div><div className="text-xs text-white/60">{cfg.device}</div></div></div>
        <div className="text-center" onDoubleClick={() => { if (window.confirm('کلید این دستگاه حذف شود؟')) { keyStore.set(''); setKey('') } }}>
          <div className="text-3xl font-extrabold tabular-nums">{timeFmt.format(now)}</div><div className="text-xs text-white/70">{dateFmt.format(now)}</div>
        </div>
        <div className="flex items-center gap-3 text-xs text-white/70">
          {pending > 0 && <span className="rounded-full bg-amber-500/20 px-2.5 py-1 text-amber-300">{fa(pending)} تردد در صف ارسال</span>}
          <span className="flex items-center gap-1">{online ? <Wifi className="h-4 w-4 text-emerald-400" /> : <WifiOff className="h-4 w-4 text-red-400" />}{online ? 'آنلاین' : 'آفلاین'}</span>
          <span className="hidden items-center gap-1 sm:flex"><Users className="h-4 w-4" />{fa(candidates.length)} چهره{galleryAt ? '' : ' (بدون دانلود)'}</span>
        </div>
      </header>

      <main className="relative mx-4 mb-3 flex-1 overflow-hidden rounded-3xl bg-black">
        {screen === 'scan' && !faceUnavailable ? (
          <>
            <video ref={videoRef} muted playsInline className="h-full w-full -scale-x-100 object-cover" />
            <svg className="pointer-events-none absolute inset-0 m-auto h-[72%] w-auto max-w-full" viewBox="0 0 100 130" aria-hidden>
              <ellipse cx="50" cy="65" rx="40" ry="55" fill="none" stroke="rgba(255,255,255,.25)" strokeWidth="1.5" />
              <ellipse cx="50" cy="65" rx="40" ry="55" fill="none" stroke="rgb(237 36 46)" strokeWidth="3" strokeLinecap="round" pathLength="100" strokeDasharray={`${progress * 100} 100`} transform="rotate(-90 50 65)" />
            </svg>
            {(camErr || !ready) && <div className="absolute inset-0 grid place-items-center bg-black/80 p-8 text-center"><div>{camErr ? <ShieldAlert className="mx-auto mb-3 h-10 w-10 text-red-400" /> : <Loader2 className="mx-auto mb-3 h-10 w-10 animate-spin" />}<p className="max-w-sm text-sm leading-7">{camErr || 'در حال آماده‌سازی تشخیص چهره…'}</p></div></div>}
            <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 to-transparent p-5 text-center"><p className="min-h-[1.75rem] text-lg font-medium">{hint || 'برای ثبت ورود یا خروج به دوربین نگاه کنید'}</p></div>
          </>
        ) : (
          <div className="grid h-full place-items-center p-6 text-center"><div><ScanFace className="mx-auto mb-3 h-14 w-14 text-white/40" /><p className="text-lg">{faceUnavailable ? 'تشخیص چهره غیرفعال است؛ با کد پرسنلی وارد شوید.' : 'ورود با کد پرسنلی'}</p></div></div>
        )}

        {screen === 'pin' && <PinPad onCancel={() => setScreen('scan')} onDone={(g) => { setScreen('scan'); show(g) }} punchOffline={punch} />}

        {greet && (
          <div className={clsx('absolute inset-0 z-10 grid place-items-center p-6 text-center backdrop-blur-sm', greet.ok ? 'bg-emerald-950/90' : 'bg-amber-950/90')}>
            <div>
              {greet.ok ? (greet.kind === 'in' ? <LogIn className="mx-auto h-24 w-24 text-emerald-400" /> : <LogOut className="mx-auto h-24 w-24 text-sky-300" />) : <XCircle className="mx-auto h-24 w-24 text-amber-400" />}
              <h2 className="mt-4 text-4xl font-extrabold">{greet.name}</h2>
              <p className="mt-2 text-2xl">{greet.ok ? (greet.kind === 'in' ? 'ورود شما ثبت شد — روز خوبی داشته باشید' : 'خروج شما ثبت شد — به سلامت') : greet.message}</p>
              {greet.ok && <p className={clsx('mt-3 text-sm', greet.offline ? 'text-amber-300' : 'text-white/60')}>{greet.message}{' · '}{timeFmt.format(new Date())}</p>}
            </div>
          </div>)}
      </main>

      <footer className="flex items-center justify-center gap-3 px-4 pb-4">
        {cfg.pin && screen === 'scan' && <button className="flex min-h-[56px] items-center gap-2 rounded-2xl bg-white/10 px-6 text-base font-semibold hover:bg-white/20" onClick={() => setScreen('pin')}><KeyRound className="h-5 w-5" />ورود با کد پرسنلی</button>}
        <button className="flex min-h-[56px] items-center gap-2 rounded-2xl bg-white/5 px-4 text-sm text-white/70 hover:bg-white/10" onClick={() => { void loadConfig(); void refreshGallery(); void flush() }}><RefreshCw className="h-4 w-4" />به‌روزرسانی</button>
      </footer>
    </div>
  )
}

function Setup({ onSave, error }: { onSave: (k: string) => void; error?: string }) {
  const [v, setV] = useState('')
  return (
    <div className="fixed inset-0 grid place-items-center bg-neutral-950 p-6 text-white" dir="rtl">
      <div className="w-full max-w-md rounded-3xl bg-white/5 p-8">
        <img src="/logo-evinki.png" alt="" className="mx-auto mb-5 h-12 rounded bg-white px-3 py-1.5" />
        <h1 className="mb-1 text-center text-xl font-extrabold">راه‌اندازی کیوسک ورود و خروج</h1>
        <p className="mb-5 text-center text-sm leading-7 text-white/60">کلید دستگاه را از پنل مدیریت (حضور و غیاب ← دستگاه‌ها ← دستگاه جدید) بگیرید و اینجا وارد کنید.</p>
        {error && <p className="mb-3 rounded-xl bg-red-500/15 p-3 text-sm text-red-300">{error}</p>}
        <input className="mb-3 w-full rounded-xl border border-white/20 bg-black/40 px-4 py-3 text-center font-mono text-sm outline-none focus:border-red-500" dir="ltr" placeholder="device key" value={v} onChange={(e) => setV(e.target.value.trim())} />
        <button className="min-h-[48px] w-full rounded-xl bg-[#ED242E] font-bold disabled:opacity-40" disabled={v.length < 16} onClick={() => onSave(v)}>ذخیره و شروع</button>
        <p className="mt-4 text-center text-xs text-white/40">برای دوربین، این صفحه باید با https باز شود.</p>
      </div>
    </div>
  )
}

function PinPad({ onCancel, onDone, punchOffline }: { onCancel: () => void; onDone: (g: Greeting) => void; punchOffline: unknown }) {
  void punchOffline
  const [step, setStep] = useState<'code' | 'pin'>('code')
  const [code, setCode] = useState('')
  const [pin, setPin] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const cur = step === 'code' ? code : pin
  const set = (s: string) => (step === 'code' ? setCode(s) : setPin(s))
  const submit = async () => {
    setBusy(true); setErr('')
    try {
      const r = await kapi('POST', '/pin-punch', { personnel_code: code, pin, client_uuid: newUuid() })
      await rememberLastKind(r.employee_id, r.kind)
      onDone({ ok: true, name: r.name, kind: r.kind, message: 'ثبت شد' })
    } catch (e: any) { setErr(!navigator.onLine ? 'ورود با کد پرسنلی نیاز به اتصال به سرور دارد' : e.message || 'کد پرسنلی یا رمز نادرست است'); setPin('') } finally { setBusy(false) }
  }
  const press = (d: string) => set((cur + d).slice(0, step === 'code' ? 10 : 8))
  return (
    <div className="absolute inset-0 z-10 grid place-items-center bg-neutral-950/95 p-4">
      <div className="w-full max-w-xs">
        <h2 className="mb-1 text-center text-lg font-bold">{step === 'code' ? 'کد پرسنلی خود را وارد کنید' : 'رمز خود را وارد کنید'}</h2>
        <div className="mb-3 rounded-2xl bg-white/10 py-3 text-center text-3xl tracking-[.4em] tabular-nums" dir="ltr">{step === 'pin' ? '•'.repeat(pin.length) || ' ' : code ? digits(code) : ' '}</div>
        {err && <p className="mb-2 rounded-xl bg-red-500/15 p-2 text-center text-sm text-red-300">{err}</p>}
        <div className="grid grid-cols-3 gap-2" dir="ltr">
          {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((d) => <button key={d} className="min-h-[58px] rounded-2xl bg-white/10 text-2xl font-semibold active:bg-white/25" onClick={() => press(d)}>{fa(d)}</button>)}
          <button className="min-h-[58px] rounded-2xl bg-white/5 text-sm" onClick={step === 'pin' && !pin ? () => setStep('code') : onCancel}>{step === 'pin' && !pin ? 'قبلی' : 'انصراف'}</button>
          <button className="min-h-[58px] rounded-2xl bg-white/10 text-2xl font-semibold active:bg-white/25" onClick={() => press('0')}>{fa(0)}</button>
          <button className="grid min-h-[58px] place-items-center rounded-2xl bg-white/5 active:bg-white/20" onClick={() => set(cur.slice(0, -1))} aria-label="پاک کردن"><Delete className="h-6 w-6" /></button>
        </div>
        <button className="mt-3 flex min-h-[56px] w-full items-center justify-center gap-2 rounded-2xl bg-[#ED242E] text-lg font-bold disabled:opacity-40" disabled={busy || (step === 'code' ? code.length < 1 : pin.length < 4)} onClick={() => (step === 'code' ? setStep('pin') : submit())}>
          {busy ? <Loader2 className="h-5 w-5 animate-spin" /> : step === 'code' ? 'ادامه' : <><CheckCircle2 className="h-5 w-5" />ثبت تردد</>}
        </button>
      </div>
    </div>
  )
}

void FACE_MODEL_NAME
