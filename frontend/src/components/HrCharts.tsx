import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, ComposedChart, Legend, Line, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { fa, faDigits } from '../lib/format'

// Colours come from the theme variables, so charts follow light/dark mode and the brand palette.
export const C = {
  present: 'rgb(var(--ok))', absent: 'rgb(var(--bad))', leave: 'rgb(37 99 235)', incomplete: 'rgb(var(--warn))',
  late: 'rgb(var(--warn))', mission: 'rgb(8 145 178)', holiday: 'rgb(var(--soft))', weekend: 'rgb(var(--line))', pending: 'rgb(var(--soft))',
  brand: 'rgb(var(--brand))', grid: 'rgb(var(--line))', text: 'rgb(var(--soft))',
}
export const STATUS_LABEL: Record<string, string> = { present: 'حاضر', absent: 'غایب', leave: 'مرخصی', mission: 'مأموریت', holiday: 'تعطیل رسمی', weekend: 'تعطیل هفتگی', incomplete: 'ناقص', pending: 'در جریان' }

const axis = { fontSize: 11, fill: C.text, fontFamily: 'Vazirmatn' }
const tip = { contentStyle: { fontFamily: 'Vazirmatn', fontSize: 12, borderRadius: 12, border: '1px solid rgb(var(--line))', background: 'rgb(var(--surface))', color: 'rgb(var(--ink))', direction: 'rtl' as const }, labelStyle: { color: 'rgb(var(--ink))' } }
const shortDay = (j: string) => faDigits((j ?? '').slice(5))

export function Box({ h = 260, children }: { h?: number; children: React.ReactElement }) {
  return <div style={{ height: h }} dir="ltr"><ResponsiveContainer width="100%" height="100%">{children}</ResponsiveContainer></div>
}

/** Per-day stacked bars: present / leave / incomplete / absent, with the number of late arrivals as a line. */
export function TrendChart({ series, h }: { series: any[]; h?: number }) {
  const names: Record<string, string> = { present: 'حاضر', leave: 'مرخصی', incomplete: 'ناقص', absent: 'غایب', late: 'تأخیر' }
  return (
    <Box h={h}>
      <ComposedChart data={series} margin={{ top: 8, right: 8, left: -12, bottom: 0 }}>
        <CartesianGrid stroke={C.grid} strokeDasharray="3 3" vertical={false} />
        <XAxis dataKey="jalali" tick={axis} tickFormatter={shortDay} interval="preserveStartEnd" minTickGap={14} />
        <YAxis tick={axis} allowDecimals={false} tickFormatter={(v) => fa(v)} />
        <Tooltip {...tip} formatter={(v, n) => [fa(v as number), names[n as string] ?? n]} labelFormatter={(l) => faDigits(l as string)} />
        <Legend formatter={(v) => <span style={{ fontFamily: 'Vazirmatn', fontSize: 12 }}>{names[v] ?? v}</span>} />
        <Bar dataKey="present" stackId="a" fill={C.present} radius={[0, 0, 0, 0]} />
        <Bar dataKey="leave" stackId="a" fill={C.leave} />
        <Bar dataKey="incomplete" stackId="a" fill={C.incomplete} />
        <Bar dataKey="absent" stackId="a" fill={C.absent} radius={[4, 4, 0, 0]} />
        <Line type="monotone" dataKey="late" stroke={C.late} strokeWidth={2} dot={false} />
      </ComposedChart>
    </Box>
  )
}

/** Smooth area chart of present vs absent (used for the 7-day dashboard trend). */
export function PresenceArea({ series, h = 260 }: { series: any[]; h?: number }) {
  return (
    <Box h={h}>
      <AreaChart data={series} margin={{ top: 8, right: 8, left: -12, bottom: 0 }}>
        <defs>
          <linearGradient id="gp" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor={C.present} stopOpacity={0.35} /><stop offset="95%" stopColor={C.present} stopOpacity={0} /></linearGradient>
          <linearGradient id="ga" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor={C.absent} stopOpacity={0.3} /><stop offset="95%" stopColor={C.absent} stopOpacity={0} /></linearGradient>
        </defs>
        <CartesianGrid stroke={C.grid} strokeDasharray="3 3" vertical={false} />
        <XAxis dataKey="jalali" tick={axis} tickFormatter={shortDay} />
        <YAxis tick={axis} allowDecimals={false} tickFormatter={(v) => fa(v)} />
        <Tooltip {...tip} formatter={(v, n) => [fa(v as number), n === 'present' ? 'حاضر' : 'غایب']} />
        <Area type="monotone" dataKey="present" stroke={C.present} strokeWidth={2} fill="url(#gp)" />
        <Area type="monotone" dataKey="absent" stroke={C.absent} strokeWidth={2} fill="url(#ga)" />
      </AreaChart>
    </Box>
  )
}

export function StatusDonut({ counters }: { counters: Record<string, number> }) {
  const data = Object.entries(counters).filter(([, v]) => v > 0).map(([k, v]) => ({ key: k, name: STATUS_LABEL[k] ?? k, value: v }))
  const total = data.reduce((a, b) => a + b.value, 0)
  if (!total) return <div className="py-12 text-center text-sm text-soft">هنوز ترددی ثبت نشده</div>
  return (
    <div>
      <Box h={190}>
        <PieChart>
          <Pie data={data} dataKey="value" nameKey="name" innerRadius={52} outerRadius={80} paddingAngle={2} stroke="none">
            {data.map((d) => <Cell key={d.key} fill={(C as any)[d.key] ?? C.holiday} />)}
          </Pie>
          <Tooltip {...tip} formatter={(v) => fa(v as number)} />
        </PieChart>
      </Box>
      <ul className="mt-2 space-y-1.5">{data.map((d) => (
        <li key={d.key} className="flex items-center justify-between text-sm"><span className="flex items-center gap-2 text-soft"><span className="h-2.5 w-2.5 rounded-full" style={{ background: (C as any)[d.key] ?? C.holiday }} />{d.name}</span><b className="num">{fa(d.value)}</b></li>))}</ul>
    </div>
  )
}

/** How many people first arrived in each hour of the day. */
export function ArrivalsChart({ arrivals, h = 220 }: { arrivals: number[]; h?: number }) {
  const rows = (arrivals ?? []).map((n, hr) => ({ hour: hr, label: fa(hr) + ':۰۰', n }))
  const used = rows.filter((r) => r.n > 0).map((r) => r.hour)
  if (!used.length) return <div className="py-12 text-center text-sm text-soft">داده‌ای نیست</div>
  const lo = Math.max(0, Math.min(...used) - 1), hi = Math.min(23, Math.max(...used) + 1)
  return (
    <Box h={h}>
      <BarChart data={rows.filter((r) => r.hour >= lo && r.hour <= hi)} margin={{ top: 8, right: 8, left: -18, bottom: 0 }}>
        <CartesianGrid stroke={C.grid} strokeDasharray="3 3" vertical={false} />
        <XAxis dataKey="label" tick={axis} />
        <YAxis tick={axis} allowDecimals={false} tickFormatter={(v) => fa(v)} />
        <Tooltip {...tip} formatter={(v) => [fa(v as number), 'ورود']} />
        <Bar dataKey="n" fill={C.brand} radius={[5, 5, 0, 0]} />
      </BarChart>
    </Box>
  )
}

/** Attendance rate per department (horizontal bars, 0–100%). */
export function DeptChart({ depts }: { depts: any[] }) {
  if (!depts?.length) return <div className="py-12 text-center text-sm text-soft">داده‌ای نیست</div>
  return (
    <Box h={Math.max(160, depts.length * 44 + 30)}>
      <BarChart data={depts} layout="vertical" margin={{ top: 4, right: 16, left: 8, bottom: 0 }}>
        <CartesianGrid stroke={C.grid} strokeDasharray="3 3" horizontal={false} />
        <XAxis type="number" domain={[0, 100]} tick={axis} tickFormatter={(v) => fa(v) + '٪'} />
        <YAxis type="category" dataKey="name" width={86} tick={axis} orientation="right" />
        <Tooltip {...tip} formatter={(v) => [fa(v as number) + '٪', 'نرخ حضور']} />
        <Bar dataKey="attendance_rate" radius={[0, 5, 5, 0]}>{depts.map((d, i) => <Cell key={i} fill={d.attendance_rate >= 90 ? C.present : d.attendance_rate >= 75 ? C.incomplete : C.absent} />)}</Bar>
      </BarChart>
    </Box>
  )
}

/** Worked hours and overtime per day. */
export function HoursChart({ series, h = 240 }: { series: any[]; h?: number }) {
  const rows = series.map((s) => ({ ...s, hours: Math.round(s.worked_minutes / 6) / 10, over: Math.round(s.overtime_minutes / 6) / 10 }))
  return (
    <Box h={h}>
      <AreaChart data={rows} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
        <CartesianGrid stroke={C.grid} strokeDasharray="3 3" vertical={false} />
        <XAxis dataKey="jalali" tick={axis} tickFormatter={shortDay} interval="preserveStartEnd" minTickGap={14} />
        <YAxis tick={axis} tickFormatter={(v) => fa(v)} />
        <Tooltip {...tip} formatter={(v, n) => [fa(v as number) + ' ساعت', n === 'hours' ? 'کارکرد کل' : 'اضافه‌کار']} />
        <Area type="monotone" dataKey="hours" stroke={C.brand} fill={C.brand} fillOpacity={0.15} strokeWidth={2} />
        <Area type="monotone" dataKey="over" stroke={C.late} fill={C.late} fillOpacity={0.2} strokeWidth={2} />
      </AreaChart>
    </Box>
  )
}

/** Ranked list with an inline bar: top late / top absent / top overtime. */
export function RankList({ rows, valueKey, unit, tone = 'warn', fmt }: { rows: any[]; valueKey: string; unit: string; tone?: 'warn' | 'bad' | 'ok'; fmt?: (n: number) => string }) {
  const max = Math.max(1, ...rows.map((r) => r[valueKey] ?? 0))
  if (!rows.length) return <div className="py-8 text-center text-sm text-soft">موردی نیست</div>
  return (
    <ul className="space-y-3">{rows.map((r) => (
      <li key={r.employee_id}>
        <div className="mb-1 flex items-center justify-between text-sm"><span className="font-medium">{r.name}</span><span className="num text-soft">{fmt ? fmt(r[valueKey]) : fa(r[valueKey])} {unit}</span></div>
        <div className="h-2 overflow-hidden rounded-full bg-muted"><div className={tone === 'bad' ? 'h-full bg-bad' : tone === 'ok' ? 'h-full bg-ok' : 'h-full bg-warn'} style={{ width: `${(r[valueKey] / max) * 100}%` }} /></div>
      </li>))}</ul>
  )
}
