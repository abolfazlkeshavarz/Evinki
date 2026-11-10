import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { Star } from 'lucide-react'
import clsx from 'clsx'
import { ActionButton, Card, Field, Loading, NumInput, useToast } from '../components/ui'
import { Logo } from '../layouts'
import { get, loginPassword, post, requestOtp, verifyOtp, type UserInfo } from '../lib/api'
import { homeFor, useUser } from '../lib/auth'
import { latin } from '../lib/format'
import { useGet } from '../lib/hooks'

function Shell({ children, title, sub }: { children: React.ReactNode; title: string; sub?: string }) {
  return (
    <div className="grid min-h-screen place-items-center p-4">
      <div className="w-full max-w-md">
        <Link to="/" className="mb-6 flex justify-center"><Logo className="text-2xl" /></Link>
        <Card className="p-6">
          <h1 className="text-xl font-bold">{title}</h1>
          {sub && <p className="mb-5 mt-1 text-sm text-soft">{sub}</p>}
          {children}
        </Card>
      </div>
    </div>
  )
}

export function Login() {
  const nav = useNavigate()
  const loc = useLocation() as { state?: { from?: string } }
  const me = useUser()
  const toast = useToast()
  const [mode, setMode] = useState<'otp' | 'password'>('otp')
  const [phone, setPhone] = useState('')
  const [code, setCode] = useState('')
  const [sent, setSent] = useState<{ dev?: string } | null>(null)
  const [name, setName] = useState('')
  const [user, setUser] = useState('')
  const [pass, setPass] = useState('')
  const [mustChange, setMustChange] = useState<UserInfo | null>(null)
  const [np, setNp] = useState('')

  const done = (u: UserInfo) => {
    if (u.must_change_password) return setMustChange(u)
    nav(loc.state?.from && !loc.state.from.startsWith('/login') ? loc.state.from : homeFor(u), { replace: true })
  }
  useEffect(() => { if (me && !mustChange) nav(homeFor(me), { replace: true }) }, [me])

  if (mustChange) {
    return (
      <Shell title="گذرواژه جدید" sub="برای ادامه، یک گذرواژه تازه انتخاب کنید (حداقل ۸ نویسه).">
        <Field label="گذرواژه جدید"><input className="input" type="password" autoComplete="new-password" value={np} onChange={(e) => setNp(e.target.value)} /></Field>
        <ActionButton className="btn-primary mt-4 w-full" onClick={async () => { await post('/auth/change-password', { current_password: pass, new_password: np }); toast.ok('گذرواژه تغییر کرد؛ دوباره وارد شوید'); setMustChange(null); setPass(''); setMode('password') }}>ذخیره</ActionButton>
      </Shell>
    )
  }

  return (
    <Shell title="ورود به اوینکی" sub="مشتریان با شماره موبایل، همکاران با نام کاربری وارد می‌شوند.">
      <div className="mb-5 grid grid-cols-2 gap-1 rounded-xl bg-muted p-1" role="tablist">
        {([['otp', 'مشتری'], ['password', 'کارمند / تکنسین']] as const).map(([k, l]) => (
          <button key={k} role="tab" aria-selected={mode === k} onClick={() => setMode(k)} className={clsx('rounded-lg py-2 text-sm font-semibold', mode === k ? 'bg-surface shadow-card' : 'text-soft')}>{l}</button>
        ))}
      </div>
      {mode === 'otp' ? (
        <form className="space-y-4" onSubmit={(e) => e.preventDefault()}>
          <Field label="شماره موبایل"><NumInput placeholder="09123456789" autoComplete="tel" value={phone} onChange={setPhone} disabled={!!sent} /></Field>
          {sent ? (
            <>
              <Field label="کد ارسال‌شده"><NumInput autoFocus maxLength={5} value={code} onChange={setCode} placeholder="•••••" /></Field>
              <Field label="نام شما (اختیاری)"><input className="input" value={name} onChange={(e) => setName(e.target.value)} /></Field>
              {sent.dev && <p className="rounded-lg bg-warn/15 p-2 text-xs text-warn">حالت توسعه: کد {sent.dev}</p>}
              <ActionButton className="btn-primary w-full" onClick={async () => done(await verifyOtp(phone, code, name))}>ورود</ActionButton>
              <button type="button" className="w-full text-sm text-soft" onClick={() => { setSent(null); setCode('') }}>تغییر شماره</button>
            </>
          ) : (
            <ActionButton className="btn-primary w-full" onClick={async () => { const r = await requestOtp(phone); setSent({ dev: r.dev_code }) }}>دریافت کد</ActionButton>
          )}
        </form>
      ) : (
        <form className="space-y-4" onSubmit={(e) => e.preventDefault()}>
          <Field label="نام کاربری یا موبایل"><input className="input" autoComplete="username" value={user} onChange={(e) => setUser(latin(e.target.value))} dir="ltr" /></Field>
          <Field label="گذرواژه"><input className="input" type="password" autoComplete="current-password" value={pass} onChange={(e) => setPass(e.target.value)} dir="ltr" onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLElement).closest('form')?.querySelector<HTMLButtonElement>('button.btn-primary')?.click()} /></Field>
          <ActionButton className="btn-primary w-full" onClick={async () => done(await loginPassword(user, pass))}>ورود</ActionButton>
          <p className="text-center text-sm text-soft">تکنسین هستید و حساب ندارید؟ <Link to="/technician/apply" className="font-semibold text-brand">درخواست همکاری</Link></p>
        </form>
      )}
    </Shell>
  )
}

export function TechApply() {
  const [f, setF] = useState({ full_name: '', phone: '', password: '', national_id: '', province: '', city: '', address: '', about: '' })
  const [ok, setOk] = useState(false)
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value })
  if (ok) return <Shell title="درخواست ثبت شد"><p className="text-sm leading-7">درخواست شما برای بررسی به دفتر مرکزی ارسال شد. پس از تایید می‌توانید با شماره موبایل و گذرواژه‌ای که انتخاب کردید وارد شوید.</p><Link className="btn-primary mt-5 w-full" to="/login">ورود</Link></Shell>
  return (
    <Shell title="درخواست همکاری تکنسین" sub="پس از بررسی و تایید، حساب شما فعال می‌شود.">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="نام و نام خانوادگی"><input className="input" value={f.full_name} onChange={set('full_name')} /></Field>
        <Field label="موبایل"><NumInput value={f.phone} onChange={(v) => setF({ ...f, phone: v })} /></Field>
        <Field label="کد ملی"><NumInput value={f.national_id} onChange={(v) => setF({ ...f, national_id: v })} /></Field>
        <Field label="گذرواژه (حداقل ۸ نویسه)"><input className="input" type="password" value={f.password} onChange={set('password')} dir="ltr" /></Field>
        <Field label="استان"><input className="input" value={f.province} onChange={set('province')} /></Field>
        <Field label="شهر"><input className="input" value={f.city} onChange={set('city')} /></Field>
        <Field label="نشانی" className="sm:col-span-2"><input className="input" value={f.address} onChange={set('address')} /></Field>
        <Field label="توضیحات (سابقه کار، تخصص)" className="sm:col-span-2"><textarea className="input" rows={3} value={f.about} onChange={set('about')} /></Field>
      </div>
      <ActionButton className="btn-primary mt-5 w-full" onClick={async () => { await post('/public/technicians/apply', f); setOk(true) }}>ارسال درخواست</ActionButton>
    </Shell>
  )
}

// Public survey page, opened from the SMS link; no sign-in.
export function SurveyPublic() {
  const { token } = useParams()
  const q = useGet<any>(`/public/surveys/${token}`)
  const [ans, setAns] = useState<Record<number, { number?: number; text?: string }>>({})
  const [done, setDone] = useState(false)
  if (q.isLoading) return <Loading />
  if (q.error) return <Shell title="این پیوند معتبر نیست"><p className="text-sm text-soft">پیوند نظرسنجی پیدا نشد یا منقضی شده است.</p></Shell>
  const s = q.data
  if (done || s.answered) return <Shell title="سپاسگزاریم 🙏"><p className="text-sm text-soft">نظر شما ثبت شد. از اینکه وقت گذاشتید ممنونیم.</p></Shell>
  if (s.closed) return <Shell title={s.title}><p className="text-sm text-soft">این نظرسنجی بسته شده است.</p></Shell>
  const set = (id: number, v: { number?: number; text?: string }) => setAns({ ...ans, [id]: v })
  return (
    <Shell title={s.title} sub={s.description || (s.name ? `${s.name} عزیز، نظر شما برای ما ارزشمند است.` : undefined)}>
      <div className="space-y-6">
        {s.questions.map((qq: any) => (
          <div key={qq.id}>
            <div className="mb-2 text-sm font-semibold">{qq.text}{qq.required && <span className="text-bad"> *</span>}</div>
            {qq.kind === 'rating' && (
              <div className="flex gap-1" role="radiogroup">
                {[1, 2, 3, 4, 5].map((n) => (
                  <button key={n} role="radio" aria-checked={ans[qq.id]?.number === n} aria-label={`${n} ستاره`} onClick={() => set(qq.id, { number: n })} className="p-1">
                    <Star className={clsx('h-9 w-9 transition', (ans[qq.id]?.number ?? 0) >= n ? 'fill-warn text-warn' : 'text-line')} />
                  </button>
                ))}
              </div>
            )}
            {qq.kind === 'nps' && (
              <div className="grid grid-cols-6 gap-1.5 sm:grid-cols-11">
                {Array.from({ length: 11 }, (_, n) => (
                  <button key={n} onClick={() => set(qq.id, { number: n })} className={clsx('min-h-[44px] rounded-lg border text-sm font-semibold num', ans[qq.id]?.number === n ? 'border-brand bg-brand text-brand-ink' : 'border-line bg-surface')}>{n}</button>
                ))}
              </div>
            )}
            {qq.kind === 'yes_no' && (
              <div className="flex gap-2">{[['بله', 1], ['خیر', 0]].map(([l, v]) => <button key={l} onClick={() => set(qq.id, { number: v as number })} className={clsx('btn flex-1 border', ans[qq.id]?.number === v ? 'border-brand bg-brand-tint text-brand' : 'border-line bg-surface')}>{l}</button>)}</div>
            )}
            {qq.kind === 'choice' && (
              <div className="space-y-2">{(qq.options as string[]).map((o) => <button key={o} onClick={() => set(qq.id, { text: o })} className={clsx('btn w-full justify-start border', ans[qq.id]?.text === o ? 'border-brand bg-brand-tint text-brand' : 'border-line bg-surface')}>{o}</button>)}</div>
            )}
            {qq.kind === 'text' && <textarea className="input" rows={3} value={ans[qq.id]?.text ?? ''} onChange={(e) => set(qq.id, { text: e.target.value })} />}
          </div>
        ))}
      </div>
      <ActionButton className="btn-primary mt-6 w-full" onClick={async () => {
        await post(`/public/surveys/${token}/answers`, { answers: Object.entries(ans).map(([id, a]) => ({ question_id: +id, number: a.number, text: a.text })) })
        setDone(true)
      }}>ثبت نظر</ActionButton>
    </Shell>
  )
}

void get
