import type {
  AuthSession,
  MetricValue,
  VideoDetail,
  VideoLike,
  VideoPlay,
  VideoViewResult,
  VideoViewSession,
} from './types'

export const AUTH_STORAGE_KEY = 'bilibili-lite.auth-session'

export async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}: ${url}`)
  }
  return (await response.json()) as T
}

export async function postJson<T = unknown>(
  url: string,
  body: Record<string, unknown>,
  accessToken?: string,
): Promise<T> {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
    },
    body: JSON.stringify(body),
  })
  if (!response.ok) {
    const payload = asRecord(await response.json().catch(() => null))
    const message = readString(payload, 'message')
    throw new Error(`${response.status}${message ? ` ${message}` : ''}`)
  }
  return (await response.json()) as T
}

export async function authorizedFetch(
  url: string,
  init: RequestInit,
  session: AuthSession,
): Promise<{ response: Response; session: AuthSession }> {
  let activeSession = await ensureFreshAuthSession(session)
  let response = await fetch(url, {
    ...init,
    headers: { ...init.headers, Authorization: `Bearer ${activeSession.accessToken}` },
  })
  if (response.status === 401) {
    activeSession = await refreshAuthSession(activeSession)
    response = await fetch(url, {
      ...init,
      headers: { ...init.headers, Authorization: `Bearer ${activeSession.accessToken}` },
    })
  }
  if (!response.ok) {
    const payload = asRecord(await response.json().catch(() => null))
    throw new Error(readString(payload, 'message') || `${response.status} ${response.statusText}`)
  }
  return { response, session: activeSession }
}

export async function ensureFreshAuthSession(session: AuthSession) {
  const refreshBefore = new Date(session.expiresAt).getTime() - 60_000
  if (Date.now() < refreshBefore) return session
  return refreshAuthSession(session)
}

export async function parseJsonResponse<T>(response: Response): Promise<T> {
  return (await response.json()) as T
}

export function persistAuthSession(session: AuthSession) {
  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session))
}

export function parseJSON(value: string): unknown {
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

export function toNumber(value: MetricValue | undefined) {
  if (typeof value === 'number') return value
  if (!value) return 0
  return Number(value)
}

export function normalizeVideoDetail(value: unknown): VideoDetail {
  const record = asRecord(value)
  return {
    bvid: readString(record, 'bvid'),
    title: readString(record, 'title'),
    description: readString(record, 'description'),
    ownerName: readString(record, 'ownerName', 'owner_name'),
    ownerAvatarUrl: readString(record, 'ownerAvatarUrl', 'owner_avatar_url'),
    coverUrl: readString(record, 'coverUrl', 'cover_url'),
    durationSeconds: readMetric(record, 'durationSeconds', 'duration_seconds'),
    viewCount: readMetric(record, 'viewCount', 'view_count'),
    danmakuCount: readMetric(record, 'danmakuCount', 'danmaku_count'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
    coinCount: readMetric(record, 'coinCount', 'coin_count'),
    favoriteCount: readMetric(record, 'favoriteCount', 'favorite_count'),
    shareCount: readMetric(record, 'shareCount', 'share_count'),
    publishTime: readTimestamp(record, 'publishTime', 'publish_time'),
    tags: readArray(record, 'tags').map(String),
    ownerId: toNumber(readMetric(record, 'ownerId', 'owner_id')),
    status: readString(record, 'status'),
  }
}

export function normalizeVideoList(value: unknown): VideoDetail[] {
  return readArray(asRecord(value), 'videos').map(normalizeVideoDetail)
}

export function normalizeVideoPlay(value: unknown): VideoPlay {
  const record = asRecord(value)
  const danmaku = asRecord(record.danmaku)
  const streams = readArray(record, 'streams').map((stream) => {
    const item = asRecord(stream)
    return {
      id: readString(item, 'id'), label: readString(item, 'label'), codec: readString(item, 'codec'),
      mimeType: readString(item, 'mimeType', 'mime_type'), url: readString(item, 'url'),
      width: Number(readMetric(item, 'width')), height: Number(readMetric(item, 'height')),
      bandwidth: Number(readMetric(item, 'bandwidth')),
      defaultStream: readBoolean(item, 'defaultStream', 'default_stream'),
    }
  }).filter((stream) => stream.mimeType === 'application/dash+xml' && stream.url.endsWith('.mpd'))
  return {
    bvid: readString(record, 'bvid'), streams,
    danmaku: {
      enabled: readBoolean(danmaku, 'enabled'), format: readString(danmaku, 'format'),
      items: readArray(danmaku, 'items').map((entry) => {
        const item = asRecord(entry)
        return {
          timeSeconds: Number(readMetric(item, 'timeSeconds', 'time_seconds')),
          text: readString(item, 'text'), color: readString(item, 'color') || '#ffffff',
        }
      }),
    },
  }
}

export function normalizeAuthSession(value: unknown): AuthSession {
  const record = asRecord(value)
  const user = asRecord(record.user)
  const expiresAt = readTimestamp(record, 'expiresAt', 'expires_at')
  const refreshExpiresAt = readTimestamp(record, 'refreshExpiresAt', 'refresh_expires_at')
  const accessToken = readString(record, 'accessToken', 'access_token')
  const refreshToken = readString(record, 'refreshToken', 'refresh_token')
  if (!accessToken || !refreshToken || !expiresAt || !refreshExpiresAt || !readString(user, 'username')) {
    throw new Error('登录响应不完整')
  }
  return {
    accessToken, refreshToken, expiresAt, refreshExpiresAt,
    user: {
      id: Number(readMetric(user, 'id')), username: readString(user, 'username'),
      displayName: readString(user, 'displayName', 'display_name'),
      avatarUrl: readString(user, 'avatarUrl', 'avatar_url'), bio: readString(user, 'bio'),
    },
  }
}

export function normalizeVideoLike(value: unknown): VideoLike {
  const record = asRecord(value)
  return {
    bvid: readString(record, 'bvid'), liked: readBoolean(record, 'liked'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
  }
}

export function normalizeVideoViewSession(value: unknown): VideoViewSession {
  const record = asRecord(value)
  return {
    sessionId: readString(record, 'sessionId', 'session_id'),
    startedAt: readTimestamp(record, 'startedAt', 'started_at') || '',
  }
}

export function normalizeVideoViewResult(value: unknown): VideoViewResult {
  const record = asRecord(value)
  return {
    counted: readBoolean(record, 'counted'),
    viewCount: readMetric(record, 'viewCount', 'view_count'),
    remainingToday: toNumber(readMetric(record, 'remainingToday', 'remaining_today')),
    nextEligibleAt: readTimestamp(record, 'nextEligibleAt', 'next_eligible_at') || '',
  }
}

export function readAuthSession(): AuthSession | null {
  try {
    const stored = window.localStorage.getItem(AUTH_STORAGE_KEY)
    if (!stored) return null
    const session = normalizeAuthSession(JSON.parse(stored))
    if (new Date(session.refreshExpiresAt).getTime() <= Date.now()) {
      window.localStorage.removeItem(AUTH_STORAGE_KEY)
      return null
    }
    return session
  } catch {
    window.localStorage.removeItem(AUTH_STORAGE_KEY)
    return null
  }
}

export function toErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback
}

export function asRecord(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>
  return {}
}

export function readString(record: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string') return value
  }
  return ''
}

function refreshAuthSession(session: AuthSession) {
  return postJson<unknown>('/api/v1/auth/refresh', { refreshToken: session.refreshToken }).then(normalizeAuthSession)
}

function readMetric(record: Record<string, unknown>, ...keys: string[]): MetricValue {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'number' || typeof value === 'string') return value
  }
  return 0
}

function readBoolean(record: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'boolean') return value
  }
  return false
}

function readArray(record: Record<string, unknown>, key: string): unknown[] {
  const value = record[key]
  return Array.isArray(value) ? value : []
}

function readTimestamp(record: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string') return value
    const timestamp = asRecord(value)
    const seconds = readMetric(timestamp, 'seconds')
    if (seconds) return new Date(Number(seconds) * 1000).toISOString()
  }
  return undefined
}
