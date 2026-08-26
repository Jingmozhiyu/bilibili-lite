import type {
  AuthSession,
  AuthUser,
  DanmakuItem,
  MetricValue,
  VideoComment,
  VideoCommentPage,
  VideoCommentHistoryItem,
  VideoCommentHistoryPage,
  VideoCommentInteraction,
  VideoDetail,
  VideoEngagement,
  VideoHistoryItem,
  VideoHistoryPage,
  VideoLike,
  VideoListPage,
  VideoPlay,
  VideoSearchPage,
  VideoViewResult,
  VideoViewSession,
} from './types'

export const AUTH_STORAGE_KEY = 'bilibili-lite.auth-session'

const configuredAPIOrigin = import.meta.env.VITE_API_ORIGIN?.trim()
const apiOrigin = (configuredAPIOrigin || (import.meta.env.PROD ? 'https://bili.madenroll.com' : '')).replace(/\/$/, '')
const configuredUploadOrigin = import.meta.env.VITE_UPLOAD_ORIGIN?.trim()
const uploadOrigin = (configuredUploadOrigin || (import.meta.env.PROD ? 'https://bili-upload.madenroll.com' : apiOrigin)).replace(/\/$/, '')

let refreshInFlight: Promise<AuthSession> | null = null

export function apiURL(url: string) {
  return withOrigin(apiOrigin, url)
}

export function uploadURL(url: string) {
  return withOrigin(uploadOrigin, url)
}

export async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiURL(url), init)
  return parseAPIResponse<T>(response, url)
}

export async function postJson<T = unknown>(
  url: string,
  body: Record<string, unknown>,
  accessToken?: string,
): Promise<T> {
  return fetchJson<T>(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
    },
    body: JSON.stringify(body),
  })
}

export async function authorizedFetch(
  url: string,
  init: RequestInit,
  session: AuthSession,
): Promise<{ response: Response; session: AuthSession }> {
  let activeSession = await ensureFreshAuthSession(session)
  let response = await fetch(apiURL(url), withAuthorization(init, activeSession.accessToken))
  if (response.status === 401) {
    activeSession = await refreshAuthSession(activeSession)
    response = await fetch(apiURL(url), withAuthorization(init, activeSession.accessToken))
  }
  if (!response.ok) {
    throw await responseError(response, url)
  }
  return { response, session: activeSession }
}

export async function authorizedJson<T>(
  url: string,
  init: RequestInit,
  session: AuthSession,
): Promise<{ data: T; session: AuthSession }> {
  const result = await authorizedFetch(url, init, session)
  return { data: await parseJsonResponse<T>(result.response), session: result.session }
}

export async function restoreAuthSession(): Promise<AuthSession | null> {
  const session = readAuthSession()
  if (!session) return null
  try {
    const fresh = await ensureFreshAuthSession(session)
    persistAuthSession(fresh)
    return fresh
  } catch {
    clearAuthSession()
    return null
  }
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

export function clearAuthSession() {
  window.localStorage.removeItem(AUTH_STORAGE_KEY)
}

export function readAuthSession(): AuthSession | null {
  try {
    const stored = window.localStorage.getItem(AUTH_STORAGE_KEY)
    if (!stored) return null
    const session = normalizeAuthSession(JSON.parse(stored))
    if (new Date(session.refreshExpiresAt).getTime() <= Date.now()) {
      clearAuthSession()
      return null
    }
    return session
  } catch {
    clearAuthSession()
    return null
  }
}

export function normalizeVideoDetail(value: unknown): VideoDetail {
  const record = asRecord(value)
  return {
    bvid: readString(record, 'bvid'),
    title: readString(record, 'title'),
    description: readString(record, 'description'),
    ownerName: readString(record, 'ownerName', 'owner_name'),
    ownerAvatarUrl: apiURL(readString(record, 'ownerAvatarUrl', 'owner_avatar_url')),
    coverUrl: apiURL(readString(record, 'coverUrl', 'cover_url')),
    durationSeconds: readMetric(record, 'durationSeconds', 'duration_seconds'),
    viewCount: readMetric(record, 'viewCount', 'view_count'),
    danmakuCount: readMetric(record, 'danmakuCount', 'danmaku_count'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
    coinCount: readMetric(record, 'coinCount', 'coin_count'),
    favoriteCount: readMetric(record, 'favoriteCount', 'favorite_count'),
    shareCount: readMetric(record, 'shareCount', 'share_count'),
    commentCount: readMetric(record, 'commentCount', 'comment_count'),
    publishTime: readTimestamp(record, 'publishTime', 'publish_time'),
    tags: readArray(record, 'tags').map(String),
    ownerId: toNumber(readMetric(record, 'ownerId', 'owner_id')),
    status: normalizeVideoStatus(record.status),
    reviewReason: readString(record, 'reviewReason', 'review_reason'),
    submittedAt: readTimestamp(record, 'submittedAt', 'submitted_at'),
    reviewedAt: readTimestamp(record, 'reviewedAt', 'reviewed_at'),
  }
}

export function normalizeVideoSearch(value: unknown): VideoSearchPage {
  const record = asRecord(value)
  return {
    videos: readArray(record, 'videos').map(normalizeVideoDetail),
    nextPageToken: readString(record, 'nextPageToken', 'next_page_token'),
    totalHits: toNumber(readMetric(record, 'totalHits', 'total_hits')),
    processingTimeMs: toNumber(readMetric(record, 'processingTimeMs', 'processing_time_ms')),
  }
}

export function normalizeVideoList(value: unknown): VideoListPage {
  const record = asRecord(value)
  return {
    videos: readArray(record, 'videos').map(normalizeVideoDetail),
    nextPageToken: readString(record, 'nextPageToken', 'next_page_token'),
  }
}

export function normalizeVideoPlay(value: unknown): VideoPlay {
  const record = asRecord(value)
  const danmaku = asRecord(record.danmaku)
  const streams = readArray(record, 'streams').map((stream) => {
    const item = asRecord(stream)
    return {
      id: readString(item, 'id'), label: readString(item, 'label'), codec: readString(item, 'codec'),
      mimeType: readString(item, 'mimeType', 'mime_type'), url: apiURL(readString(item, 'url')),
      width: Number(readMetric(item, 'width')), height: Number(readMetric(item, 'height')),
      bandwidth: Number(readMetric(item, 'bandwidth')),
      defaultStream: readBoolean(item, 'defaultStream', 'default_stream'),
    }
  }).filter((stream) => stream.mimeType === 'application/dash+xml' && stream.url.endsWith('.mpd'))
  return {
    bvid: readString(record, 'bvid'), streams,
    danmaku: {
      enabled: readBoolean(danmaku, 'enabled'), format: readString(danmaku, 'format'),
      items: readArray(danmaku, 'items').map(normalizeDanmaku),
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
    user: normalizeUser(user),
  }
}

export function normalizeUser(value: unknown): AuthUser {
  const user = asRecord(value)
  return {
    id: Number(readMetric(user, 'id')), username: readString(user, 'username'),
    displayName: readString(user, 'displayName', 'display_name'),
    avatarUrl: apiURL(readString(user, 'avatarUrl', 'avatar_url')), bio: readString(user, 'bio'),
    coinBalance: toNumber(readMetric(user, 'coinBalance', 'coin_balance')),
    isAdmin: readBoolean(user, 'isAdmin', 'is_admin'),
    experience: Math.max(0, toNumber(readMetric(user, 'experience'))),
    level: Math.min(6, Math.max(0, toNumber(readMetric(user, 'level')))),
  }
}

export function normalizeVideoLike(value: unknown): VideoLike {
  const record = asRecord(value)
  return {
    bvid: readString(record, 'bvid'), liked: readBoolean(record, 'liked'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
  }
}

export function normalizeVideoEngagement(value: unknown): VideoEngagement {
  const record = asRecord(value)
  return {
    bvid: readString(record, 'bvid'), liked: readBoolean(record, 'liked'),
    favorited: readBoolean(record, 'favorited'),
    myCoinAmount: toNumber(readMetric(record, 'myCoinAmount', 'my_coin_amount')),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
    favoriteCount: readMetric(record, 'favoriteCount', 'favorite_count'),
    coinCount: readMetric(record, 'coinCount', 'coin_count'),
    shareCount: readMetric(record, 'shareCount', 'share_count'),
    coinBalance: toNumber(readMetric(record, 'coinBalance', 'coin_balance')),
  }
}

export function normalizeVideoHistory(value: unknown): VideoHistoryPage {
  const record = asRecord(value)
  return {
    items: readArray(record, 'items').map((entry): VideoHistoryItem => {
      const item = asRecord(entry)
      return {
        video: normalizeVideoDetail(item.video),
        interactedAt: readTimestamp(item, 'interactedAt', 'interacted_at'),
        coinAmount: toNumber(readMetric(item, 'coinAmount', 'coin_amount')),
      }
    }),
    nextPageToken: readString(record, 'nextPageToken', 'next_page_token'),
  }
}

export function normalizeVideoComment(value: unknown): VideoComment {
  const record = asRecord(value)
  return {
    id: toNumber(readMetric(record, 'id')), bvid: readString(record, 'bvid'),
    userId: toNumber(readMetric(record, 'userId', 'user_id')),
    userName: readString(record, 'userName', 'user_name'),
    userAvatarUrl: apiURL(readString(record, 'userAvatarUrl', 'user_avatar_url')),
    content: readString(record, 'content'), createdAt: readTimestamp(record, 'createdAt', 'created_at'),
    rootId: toNumber(readMetric(record, 'rootId', 'root_id')),
    parentId: toNumber(readMetric(record, 'parentId', 'parent_id')),
    replyToUserId: toNumber(readMetric(record, 'replyToUserId', 'reply_to_user_id')),
    replyToUserName: readString(record, 'replyToUserName', 'reply_to_user_name'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
    liked: readBoolean(record, 'liked'),
    replyCount: readMetric(record, 'replyCount', 'reply_count'),
    deleted: readBoolean(record, 'deleted'),
  }
}

export function normalizeVideoCommentInteraction(value: unknown): VideoCommentInteraction {
  const record = asRecord(value)
  return {
    commentId: toNumber(readMetric(record, 'commentId', 'comment_id')),
    liked: readBoolean(record, 'liked'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
  }
}

export function normalizeVideoCommentHistory(value: unknown): VideoCommentHistoryPage {
  const record = asRecord(value)
  return {
    items: readArray(record, 'items').map((entry): VideoCommentHistoryItem => {
      const item = asRecord(entry)
      return { video: normalizeVideoDetail(item.video), comment: normalizeVideoComment(item.comment) }
    }),
    nextPageToken: readString(record, 'nextPageToken', 'next_page_token'),
  }
}

export function normalizeVideoComments(value: unknown): VideoCommentPage {
  const record = asRecord(value)
  return {
    comments: readArray(record, 'comments').map(normalizeVideoComment),
    nextPageToken: readString(record, 'nextPageToken', 'next_page_token'),
  }
}

export function normalizeDanmaku(value: unknown): DanmakuItem {
  const record = asRecord(value)
  return {
    id: toNumber(readMetric(record, 'id')),
    userId: toNumber(readMetric(record, 'userId', 'user_id')),
    userName: readString(record, 'userName', 'user_name'),
    timeSeconds: Number(readMetric(record, 'timeSeconds', 'time_seconds')),
    text: readString(record, 'text'), color: readString(record, 'color') || '#ffffff',
    createdAt: readTimestamp(record, 'createdAt', 'created_at'),
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

function normalizeVideoStatus(value: unknown) {
  const status = typeof value === 'number' || typeof value === 'string' ? String(value) : ''
  const numericStatuses: Record<string, string> = {
    '1': 'processing',
    '2': 'ready',
    '3': 'pending_review',
    '4': 'published',
    '5': 'rejected',
    '6': 'failed',
    '7': 'deleted',
  }
  return numericStatuses[status] ?? status.replace(/^VIDEO_STATUS_/, '').toLowerCase()
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

async function refreshAuthSession(session: AuthSession) {
  if (refreshInFlight) return refreshInFlight
  if (new Date(session.refreshExpiresAt).getTime() <= Date.now()) {
    throw new Error('登录状态已过期')
  }
  const request = postJson<unknown>('/api/v1/auth/refresh', { refresh_token: session.refreshToken })
    .then(normalizeAuthSession)
    .then((fresh) => {
      persistAuthSession(fresh)
      return fresh
    })
    .finally(() => {
      refreshInFlight = null
    })
  refreshInFlight = request
  return request
}

function withAuthorization(init: RequestInit, accessToken: string): RequestInit {
  return {
    ...init,
    headers: { ...init.headers, Authorization: `Bearer ${accessToken}` },
  }
}

function withOrigin(origin: string, url: string) {
  if (!origin || !url.startsWith('/')) return url
  return `${origin}${url}`
}

async function parseAPIResponse<T>(response: Response, url: string): Promise<T> {
  if (!response.ok) throw await responseError(response, url)
  if (response.status === 204) return undefined as T
  const text = await response.text()
  return (text ? JSON.parse(text) : undefined) as T
}

async function responseError(response: Response, fallback: string) {
  const payload = asRecord(await response.json().catch(() => null))
  const message = readString(payload, 'message')
  return new Error(`${response.status}${message ? ` ${message}` : ` ${response.statusText}: ${fallback}`}`)
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
