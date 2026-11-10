/**
 * صف محلی ترددها روی تبلت.
 *
 * وقتی اینترنت قطع است، هر تردد با یک شناسه یکتا (`client_uuid`) در IndexedDB
 * ذخیره می‌شود. به‌محض برقراری ارتباط، همه رکوردهای در صف به‌صورت دسته‌ای به
 * `/kiosk/sync` فرستاده می‌شوند. چون سرور بر اساس همان شناسه یکتا، رکورد تکراری
 * نمی‌سازد، ارسال مجدد یک بسته کاملاً بی‌خطر است.
 *
 * گالری چهره‌ها هم اینجا نگهداری می‌شود تا تبلت پس از ری‌استارت، بدون اینترنت
 * همچنان بتواند پرسنل را بشناسد.
 */
import { openDB, type IDBPDatabase } from 'idb'
export interface FaceGallery { version: string; items: { employee_id: number; name: string; personnel_code: string; vectors: number[][] }[] }

const DB_NAME = 'evinki-kiosk'
const DB_VERSION = 1
const QUEUE_STORE = 'punch-queue'
const META_STORE = 'meta'

export interface QueuedPunch {
  client_uuid: string
  employee_id: number
  employee_name: string
  kind: 'in' | 'out'
  method: 'face' | 'pin'
  happened_at: string
  confidence?: number | null
  snapshot_base64?: string | null
  created_at: number
  attempts: number
}

let dbPromise: Promise<IDBPDatabase> | null = null

function db() {
  dbPromise ??= openDB(DB_NAME, DB_VERSION, {
    upgrade(database) {
      if (!database.objectStoreNames.contains(QUEUE_STORE)) {
        database.createObjectStore(QUEUE_STORE, { keyPath: 'client_uuid' })
      }
      if (!database.objectStoreNames.contains(META_STORE)) {
        database.createObjectStore(META_STORE)
      }
    },
  })
  return dbPromise
}

export function newUuid(): string {
  if (crypto.randomUUID) return crypto.randomUUID().replace(/-/g, '')
  return Array.from(crypto.getRandomValues(new Uint8Array(16)))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

// ------------------------------------------------------------------ صف تردد

export async function enqueue(punch: Omit<QueuedPunch, 'created_at' | 'attempts'>) {
  const conn = await db()
  await conn.put(QUEUE_STORE, { ...punch, created_at: Date.now(), attempts: 0 })
}

export async function queued(): Promise<QueuedPunch[]> {
  const conn = await db()
  const all = (await conn.getAll(QUEUE_STORE)) as QueuedPunch[]
  return all.sort((a, b) => a.created_at - b.created_at)
}

export async function queueSize(): Promise<number> {
  const conn = await db()
  return conn.count(QUEUE_STORE)
}

export async function removeMany(uuids: string[]) {
  if (uuids.length === 0) return
  const conn = await db()
  const tx = conn.transaction(QUEUE_STORE, 'readwrite')
  await Promise.all(uuids.map((id) => tx.store.delete(id)))
  await tx.done
}

export async function markAttempt(uuids: string[]) {
  const conn = await db()
  const tx = conn.transaction(QUEUE_STORE, 'readwrite')
  for (const id of uuids) {
    const item = (await tx.store.get(id)) as QueuedPunch | undefined
    if (item) await tx.store.put({ ...item, attempts: item.attempts + 1 })
  }
  await tx.done
}

export async function clearQueue() {
  const conn = await db()
  await conn.clear(QUEUE_STORE)
}

// -------------------------------------------------------------- گالری چهره‌ها

export async function saveGallery(gallery: FaceGallery) {
  const conn = await db()
  await conn.put(META_STORE, gallery, 'gallery')
  await conn.put(META_STORE, Date.now(), 'gallery_saved_at')
}

export async function loadGallery(): Promise<FaceGallery | null> {
  const conn = await db()
  return ((await conn.get(META_STORE, 'gallery')) as FaceGallery) ?? null
}

export async function gallerySavedAt(): Promise<number | null> {
  const conn = await db()
  return ((await conn.get(META_STORE, 'gallery_saved_at')) as number) ?? null
}

/** آخرین تردد هر پرسنل روی همین دستگاه — برای تشخیص «نوبت ورود است یا خروج». */
export async function rememberLastKind(employeeId: number, kind: 'in' | 'out') {
  const conn = await db()
  const map = ((await conn.get(META_STORE, 'last_kind')) as Record<string, { kind: string; at: number }>) ?? {}
  map[String(employeeId)] = { kind, at: Date.now() }
  await conn.put(META_STORE, map, 'last_kind')
}

export async function lastKindFor(employeeId: number): Promise<'in' | 'out' | null> {
  const conn = await db()
  const map = ((await conn.get(META_STORE, 'last_kind')) as Record<string, { kind: string; at: number }>) ?? {}
  const entry = map[String(employeeId)]
  if (!entry) return null
  // اگر بیش از ۱۸ ساعت گذشته، شیفت جدید است و دوباره «ورود» می‌شود
  if (Date.now() - entry.at > 18 * 3600_000) return null
  return entry.kind as 'in' | 'out'
}
