import { useState, type ReactNode } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { ActionButton, Card, Col, DataTable, Empty, ErrorBox, Field, ImageUpload, JalaliInput, Loading, Modal, MoneyInput, NumInput, PageHeader, Pager, SearchBox } from './ui'
import { CodeFormulaBuilder } from './CodeFormula'
import { del, patch, post } from '../lib/api'
import { qs, useGet, useInvalidate } from '../lib/hooks'
import { useCan } from '../lib/auth'

export type FieldDef = {
  key: string
  label: string
  type?: 'text' | 'textarea' | 'int' | 'money' | 'bool' | 'select' | 'jdate' | 'image' | 'images' | 'kv' | 'time' | 'password' | 'formula'
  options?: { value: string | number; label: string }[]
  hint?: string
  half?: boolean
  required?: boolean
  create?: boolean // only when creating
}

/** Renders one input for a field definition. */
export function FieldInput({ f, value, onChange }: { f: FieldDef; value: any; onChange: (v: any) => void }) {
  switch (f.type) {
    case 'textarea':
      return <textarea className="input" rows={3} value={value ?? ''} onChange={(e) => onChange(e.target.value)} />
    case 'int':
      return <NumInput value={value ?? ''} onChange={(s) => onChange(s === '' ? null : Number(s))} />
    case 'money':
      return <MoneyInput value={value ?? 0} onChange={onChange} />
    case 'bool':
      return <label className="flex min-h-[44px] items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4" checked={!!value} onChange={(e) => onChange(e.target.checked)} />فعال</label>
    case 'select':
      return <select className="input" value={value ?? ''} onChange={(e) => onChange(isNaN(+e.target.value) || e.target.value === '' ? e.target.value : +e.target.value)}>
        <option value="">—</option>{f.options?.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}</select>
    case 'jdate':
      return <JalaliInput value={value ?? ''} onChange={onChange} />
    case 'image':
      return <ImageUpload value={value} onChange={onChange} />
    case 'images':
      return (
        <div className="space-y-2">
          {(value ?? []).map((u: string, i: number) => <div key={i} className="flex items-center gap-2"><img src={u} alt="" className="h-12 w-12 rounded-lg object-cover" /><button type="button" className="text-xs text-bad" onClick={() => onChange(value.filter((_: string, j: number) => j !== i))}>حذف</button></div>)}
          <ImageUpload value="" onChange={(u) => u && onChange([...(value ?? []), u])} />
        </div>
      )
    case 'kv': {
      const entries: [string, string][] = Object.entries(value ?? {}) as any
      const set = (e: [string, string][]) => onChange(Object.fromEntries(e.filter(([k]) => k)))
      return (
        <div className="space-y-2">
          {entries.map(([k, v], i) => (
            <div key={i} className="flex gap-2"><input className="input" placeholder="ویژگی" value={k} onChange={(e) => { const n = [...entries]; n[i] = [e.target.value, v]; set(n) }} /><input className="input" placeholder="مقدار" value={v} onChange={(e) => { const n = [...entries]; n[i] = [k, e.target.value]; set(n) }} /></div>
          ))}
          <button type="button" className="btn-ghost btn-sm" onClick={() => onChange({ ...(value ?? {}), ['ویژگی ' + (entries.length + 1)]: '' })}><Plus className="h-4 w-4" />افزودن ویژگی</button>
        </div>
      )
    }
    case 'formula':
      return <CodeFormulaBuilder value={value} onChange={onChange} />
    case 'time':
      return <input className="input num" dir="ltr" type="time" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />
    case 'password':
      return <input className="input" type="password" autoComplete="new-password" value={value ?? ''} onChange={(e) => onChange(e.target.value)} dir="ltr" />
    default:
      return <input className="input" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />
  }
}

export function FormFields({ fields, value, onChange, creating }: { fields: FieldDef[]; value: any; onChange: (v: any) => void; creating?: boolean }) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {fields.filter((f) => creating || !f.create).map((f) => (
        <Field key={f.key} label={f.label + (f.required ? ' *' : '')} hint={f.hint} className={f.half ? '' : 'sm:col-span-2'}>
          <FieldInput f={f} value={value[f.key]} onChange={(v) => onChange({ ...value, [f.key]: v })} />
        </Field>
      ))}
    </div>
  )
}

/**
 * A complete list + create/edit/delete screen from a declaration.
 * Server-side search, paging and permissions come from the /admin/... CRUD endpoints.
 */
export function ResourcePage({ title, subtitle, path, fields, columns, empty, filters, canWrite, canDelete, extraActions, rowActions, pageSize = 20, searchable = true, newRow, query }: {
  title: string; subtitle?: string; path: string; fields: FieldDef[]; columns: Col<any>[]
  empty?: any; filters?: ReactNode; canWrite?: string[]; canDelete?: string[]; extraActions?: ReactNode
  rowActions?: (r: any) => ReactNode; pageSize?: number; searchable?: boolean; newRow?: any; query?: Record<string, string | number | undefined>
}) {
  const can = useCan()
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [edit, setEdit] = useState<any>(null)
  const inv = useInvalidate()
  const list = useGet<any>(path + qs({ q, page, limit: pageSize, ...query }))
  const writable = !canWrite || can(...canWrite)
  const deletable = !!canDelete && can(...canDelete)
  const save = async () => {
    const body: any = {}
    for (const f of fields) if (edit[f.key] !== undefined && (!f.create || !edit.id)) body[f.key] = edit[f.key]
    edit.id ? await patch(`${path}/${edit.id}`, body) : await post(path, body)
    inv(path)
    setEdit(null)
  }
  const cols: Col<any>[] = [
    ...columns,
    ...(writable || deletable || rowActions ? [{ head: '', cell: (r: any) => (
      <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
        {rowActions?.(r)}
        {writable && <button className="btn-ghost btn-sm" onClick={() => setEdit(r)}>ویرایش</button>}
        {deletable && <ActionButton className="btn-ghost btn-sm text-bad" confirm="حذف شود؟" onClick={async () => { await del(`${path}/${r.id}`); inv(path) }}><Trash2 className="h-4 w-4" /></ActionButton>}
      </div>) } as Col<any>] : []),
  ]
  return (
    <div>
      <PageHeader title={title} subtitle={subtitle} actions={<>{extraActions}{writable && <button className="btn-primary" onClick={() => setEdit({ ...(newRow ?? {}) })}><Plus className="h-4 w-4" />افزودن</button>}</>} />
      <Card>
        {(searchable || filters) && <div className="flex flex-wrap items-center gap-2 border-b border-line p-3">{searchable && <SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} />}{filters}</div>}
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} retry={() => list.refetch()} /> : !list.data.items.length ? (empty ?? <Empty />) : (
          <>
            <DataTable rows={list.data.items} cols={cols} rowKey={(r) => r.id} onRow={writable ? setEdit : undefined} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
      <Modal open={!!edit} onClose={() => setEdit(null)} title={edit?.id ? 'ویرایش' : 'افزودن'} wide footer={<><button className="btn-ghost" onClick={() => setEdit(null)}>انصراف</button><ActionButton onClick={save}>ذخیره</ActionButton></>}>
        {edit && <FormFields fields={fields} value={edit} onChange={setEdit} creating={!edit.id} />}
      </Modal>
    </div>
  )
}
