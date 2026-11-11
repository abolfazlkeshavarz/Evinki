// Money, numbers and Solar Hijri dates. Dates use the browser's own Intl calendar,
// so there is no calendar library to maintain.

const nf = new Intl.NumberFormat('fa-IR')
export const fa = (n: number | string | null | undefined) => (n === null || n === undefined || n === '' ? '—' : nf.format(Number(n)))
export const toman = (n: number | null | undefined) => (n === null || n === undefined ? '—' : nf.format(n) + ' تومان')

const persianDigits = '۰۱۲۳۴۵۶۷۸۹'
const arabicDigits = '٠١٢٣٤٥٦٧٨٩'
/** Accepts Persian/Arabic digits anywhere and returns ASCII digits. */
export function latin(s: string): string {
  return s.replace(/[۰-۹]/g, (d) => String(persianDigits.indexOf(d))).replace(/[٠-٩]/g, (d) => String(arabicDigits.indexOf(d)))
}
/** Parse typed money like "۱٬۲۵۰٬۰۰۰" → 1250000. */
export function parseMoney(s: string): number {
  return Number(latin(s).replace(/[^\d]/g, '')) || 0
}

const TZ = 'Asia/Tehran'
const jf = new Intl.DateTimeFormat('fa-IR-u-ca-persian', { year: 'numeric', month: '2-digit', day: '2-digit', timeZone: TZ })
const jfLong = new Intl.DateTimeFormat('fa-IR-u-ca-persian', { year: 'numeric', month: 'long', day: 'numeric', timeZone: TZ })
const jfTime = new Intl.DateTimeFormat('fa-IR-u-ca-persian', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: TZ })
const parts = new Intl.DateTimeFormat('en-u-ca-persian-nu-latn', { year: 'numeric', month: 'numeric', day: 'numeric', timeZone: 'Asia/Tehran' })

function asDate(v: string | Date | null | undefined): Date | null {
  if (!v) return null
  const d = v instanceof Date ? v : new Date(v)
  return isNaN(d.getTime()) ? null : d
}
export const date = (v?: string | Date | null) => {
  const d = asDate(v)
  return d ? jf.format(d) : '—'
}
export const dateLong = (v?: string | Date | null) => {
  const d = asDate(v)
  return d ? jfLong.format(d) : '—'
}
export const dateTime = (v?: string | Date | null) => {
  const d = asDate(v)
  return d ? jfTime.format(d) : '—'
}

/** Solar Hijri parts of a Date. */
export function jalaliParts(d: Date): { y: number; m: number; d: number } {
  const p = Object.fromEntries(parts.formatToParts(d).map((x) => [x.type, x.value]))
  return { y: +p.year, m: +p.month, d: +p.day }
}

/** Solar Hijri → ISO date (YYYY-MM-DD). Walks from an estimate until Intl agrees. */
export function jalaliToISO(jy: number, jm: number, jd: number): string | null {
  if (jm < 1 || jm > 12 || jd < 1 || jd > 31) return null
  let guess = new Date(Date.UTC(jy + 621, 2, 21, 12) + ((jm - 1) * 30.4 + (jd - 1)) * 86400000)
  for (let i = 0; i < 8; i++) {
    const p = jalaliParts(guess)
    if (p.y === jy && p.m === jm && p.d === jd) return guess.toISOString().slice(0, 10)
    const diff = (jy - p.y) * 365.25 + (jm - p.m) * 30.4 + (jd - p.d)
    guess = new Date(guess.getTime() + Math.round(diff) * 86400000 || (diff > 0 ? 86400000 : -86400000))
  }
  return null
}

/** "1405/03/17" (any digits) → ISO date, or null when it is not a real date. */
export function parseJalali(s: string): string | null {
  const m = latin(s.trim()).match(/^(\d{4})[/\-.](\d{1,2})[/\-.](\d{1,2})$/)
  return m ? jalaliToISO(+m[1], +m[2], +m[3]) : null
}
export function isoToJalaliInput(iso?: string | null): string {
  const d = asDate(iso ? iso.slice(0, 10) + 'T12:00:00Z' : null)
  if (!d) return ''
  const p = jalaliParts(d)
  return `${p.y}/${String(p.m).padStart(2, '0')}/${String(p.d).padStart(2, '0')}`
}
// today in Tehran (toISOString() would still say yesterday between midnight and 03:30)
const tehranDay = new Intl.DateTimeFormat('en-CA', { timeZone: TZ })
export const todayISO = () => tehranDay.format(new Date())
export const addMonthsISO = (iso: string, months: number) => {
  const d = new Date(iso + 'T12:00:00Z')
  d.setUTCMonth(d.getUTCMonth() + months)
  return d.toISOString().slice(0, 10)
}

export function ago(v?: string | null): string {
  const d = asDate(v)
  if (!d) return '—'
  const s = (Date.now() - d.getTime()) / 1000
  if (s < 60) return 'همین الان'
  if (s < 3600) return fa(Math.floor(s / 60)) + ' دقیقه پیش'
  if (s < 86400) return fa(Math.floor(s / 3600)) + ' ساعت پیش'
  if (s < 86400 * 30) return fa(Math.floor(s / 86400)) + ' روز پیش'
  return date(d)
}

export function minutes(m: number): string {
  if (!m) return '—'
  const h = Math.floor(m / 60)
  return (h ? fa(h) + ' ساعت و ' : '') + fa(m % 60) + ' دقیقه'
}

/** Persian digits for text that is not a plain number (times, dates, codes with leading zeros). */
export const faDigits = (s: string | number | null | undefined) => String(s ?? '').replace(/\d/g, (d) => '۰۱۲۳۴۵۶۷۸۹'[+d])

/** ISO date + n Solar Hijri months (day kept, clamped to the month length), the same rule the server uses for guarantees. */
export function addJalaliMonths(iso: string, n: number): string {
  const p = jalaliParts(new Date(iso + 'T12:00:00Z'))
  const t = p.y * 12 + (p.m - 1) + n
  const y = Math.floor(t / 12), m = (t % 12) + 1
  for (let d = p.d; d >= 1; d--) { const r = jalaliToISO(y, m, d); if (r) return r }
  return iso
}
