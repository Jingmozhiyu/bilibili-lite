import { useEffect, useMemo, useRef, useState } from 'react'
import './App.css'

type MetricValue = number | string

type VideoDetail = {
  bvid: string
  title: string
  description: string
  ownerName: string
  ownerAvatarUrl?: string
  coverUrl?: string
  durationSeconds: MetricValue
  viewCount: MetricValue
  danmakuCount: MetricValue
  likeCount: MetricValue
  coinCount: MetricValue
  favoriteCount: MetricValue
  shareCount: MetricValue
  publishTime?: string
  tags: string[]
}

type VideoStream = {
  id: string
  label: string
  codec: string
  mimeType: string
  url: string
  width: number
  height: number
  bandwidth: number
  defaultStream: boolean
}

type DanmakuItem = {
  timeSeconds: number
  text: string
  color: string
}

type VideoPlay = {
  bvid: string
  streams: VideoStream[]
  danmaku?: {
    enabled: boolean
    format: string
    items: DanmakuItem[]
  }
}

type LoadState =
  | { status: 'loading' }
  | { status: 'ready'; detail: VideoDetail; play: VideoPlay }
  | { status: 'error'; message: string }

type AuthUser = {
  id: number
  username: string
  displayName: string
  avatarUrl?: string
  bio?: string
}

type AuthSession = {
  accessToken: string
  expiresAt: string
  user: AuthUser
}

const FALLBACK_BVID = 'BV1'
const AUTH_STORAGE_KEY = 'bilibili-lite.auth-session'

function App() {
  const bvid = getBvidFromPath()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [selectedStreamId, setSelectedStreamId] = useState<string>('')
  const [danmakuOn, setDanmakuOn] = useState(true)
  const [currentTime, setCurrentTime] = useState(0)
  const videoRef = useRef<HTMLVideoElement | null>(null)

  useEffect(() => {
    let ignore = false

    Promise.all([
      fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}`),
      fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/play`),
    ])
      .then(([detailResponse, playResponse]) => {
        if (ignore) return
        const detail = normalizeVideoDetail(detailResponse)
        const play = normalizeVideoPlay(playResponse)
        setState({ status: 'ready', detail, play })
        const defaultStream =
          play.streams.find((stream) => stream.defaultStream) ?? play.streams[0]
        setSelectedStreamId(defaultStream?.id ?? '')
      })
      .catch((error: Error) => {
        if (!ignore) {
          setState({ status: 'error', message: error.message })
        }
      })

    return () => {
      ignore = true
    }
  }, [bvid])

  const selectedStream = useMemo(() => {
    if (state.status !== 'ready') return undefined
    return (
      state.play.streams.find((stream) => stream.id === selectedStreamId) ??
      state.play.streams[0]
    )
  }, [selectedStreamId, state])

  const activeDanmaku = useMemo(() => {
    if (state.status !== 'ready' || !danmakuOn) return []
    return (state.play.danmaku?.items ?? []).filter((item) => {
      const age = currentTime - item.timeSeconds
      return age >= 0 && age <= 4.8
    })
  }, [currentTime, danmakuOn, state])

  function switchStream(streamId: string) {
    const video = videoRef.current
    const time = video?.currentTime ?? 0
    const wasPlaying = video ? !video.paused : false
    setSelectedStreamId(streamId)
    window.setTimeout(() => {
      const nextVideo = videoRef.current
      if (!nextVideo) return
      nextVideo.currentTime = time
      if (wasPlaying) {
        void nextVideo.play()
      }
    }, 0)
  }

  if (state.status === 'loading') {
    return <LoadingPage bvid={bvid} />
  }

  if (state.status === 'error') {
    return <ErrorPage bvid={bvid} message={state.message} />
  }

  const { detail, play } = state

  return (
    <main className="page-shell">
      <header className="topbar" aria-label="主导航">
        <a className="brand" href={`/video/${FALLBACK_BVID}`}>
          <span className="brand-mark">b</span>
          <span>bilibili-lite</span>
        </a>
        <div className="search-box" role="search">
          <input aria-label="搜索视频" placeholder="搜索视频、番剧、直播" />
          <button type="button">搜索</button>
        </div>
        <div className="topbar-actions">
          <nav className="quick-links" aria-label="快捷入口">
            <a href={`/video/${detail.bvid}`}>首页</a>
            <a href={`/video/${detail.bvid}`}>动态</a>
            <a href={`/video/${detail.bvid}`}>收藏</a>
          </nav>
          <AuthMenu />
        </div>
      </header>

      <section className="watch-layout">
        <div className="primary-column">
          <section className="player-panel" aria-label="视频播放器">
            <div className="player-stage">
              {selectedStream ? (
                <video
                  ref={videoRef}
                  className="player-video"
                  src={selectedStream.url}
                  poster={detail.coverUrl || undefined}
                  controls
                  autoPlay
                  muted
                  playsInline
                  preload="metadata"
                  onTimeUpdate={(event) =>
                    setCurrentTime(event.currentTarget.currentTime)
                  }
                />
              ) : (
                <div className="player-empty">没有可用的视频资源</div>
              )}

              {activeDanmaku.length > 0 && (
                <div className="danmaku-layer" aria-hidden="true">
                  {activeDanmaku.map((item, index) => (
                    <span
                      key={`${item.timeSeconds}-${item.text}`}
                      className="danmaku-item"
                      style={{
                        color: item.color,
                        top: `${14 + index * 16}%`,
                        animationDelay: `${index * 120}ms`,
                      }}
                    >
                      {item.text}
                    </span>
                  ))}
                </div>
              )}
            </div>

            <div className="player-toolbar">
              <div className="quality-group" aria-label="清晰度">
                {play.streams.map((stream) => (
                  <button
                    key={stream.id}
                    type="button"
                    className={stream.id === selectedStream?.id ? 'active' : ''}
                    onClick={() => switchStream(stream.id)}
                  >
                    {stream.label}
                  </button>
                ))}
              </div>
              <button
                type="button"
                className={danmakuOn ? 'toggle active' : 'toggle'}
                onClick={() => setDanmakuOn((value) => !value)}
                aria-pressed={danmakuOn}
              >
                弹幕 {danmakuOn ? '开' : '关'}
              </button>
              {selectedStream && (
                <span className="stream-note">
                  {selectedStream.codec.toUpperCase()} ·{' '}
                  {formatBitrate(selectedStream.bandwidth)}
                </span>
              )}
            </div>
          </section>

          <section className="video-meta" aria-labelledby="video-title">
            <h1 id="video-title">{detail.title}</h1>
            <div className="meta-row">
              <span>{formatCount(detail.viewCount)} 播放</span>
              <span>{formatCount(detail.danmakuCount)} 弹幕</span>
              <span>{formatDate(detail.publishTime)}</span>
              <span>{detail.bvid}</span>
            </div>
            <p>{detail.description}</p>
            <div className="tag-list" aria-label="视频标签">
              {detail.tags.map((tag) => (
                <span key={tag}>{tag}</span>
              ))}
            </div>
          </section>
        </div>

        <aside className="side-column" aria-label="视频信息">
          <section className="owner-block">
            <div className="avatar" aria-hidden="true">
              {detail.ownerAvatarUrl ? (
                <img src={detail.ownerAvatarUrl} alt="" />
              ) : (
                detail.ownerName.slice(0, 1).toUpperCase()
              )}
            </div>
            <div>
              <strong>{detail.ownerName}</strong>
              <span>本地视频源维护者</span>
            </div>
            <button type="button">关注</button>
          </section>

          <section className="action-grid" aria-label="互动数据">
            <Metric label="点赞" value={detail.likeCount} />
            <Metric label="投币" value={detail.coinCount} />
            <Metric label="收藏" value={detail.favoriteCount} />
            <Metric label="分享" value={detail.shareCount} />
          </section>

          <section className="play-info">
            <h2>播放信息</h2>
            <dl>
              <div>
                <dt>资源</dt>
                <dd>{selectedStream?.url ?? '-'}</dd>
              </div>
              <div>
                <dt>格式</dt>
                <dd>{selectedStream?.mimeType ?? '-'}</dd>
              </div>
              <div>
                <dt>分辨率</dt>
                <dd>
                  {selectedStream
                    ? `${selectedStream.width} x ${selectedStream.height}`
                    : '-'}
                </dd>
              </div>
              <div>
                <dt>弹幕</dt>
                <dd>{play.danmaku?.items.length ?? 0} 条预载</dd>
              </div>
            </dl>
          </section>
        </aside>
      </section>
    </main>
  )
}

function LoadingPage({ bvid }: { bvid: string }) {
  return (
    <main className="page-shell">
      <div className="loading-grid" aria-label={`正在加载 ${bvid}`}>
        <div className="skeleton player-skeleton" />
        <div className="skeleton text-skeleton wide" />
        <div className="skeleton text-skeleton" />
        <div className="skeleton side-skeleton" />
      </div>
    </main>
  )
}

function ErrorPage({ bvid, message }: { bvid: string; message: string }) {
  return (
    <main className="page-shell">
      <section className="error-panel">
        <span>{bvid}</span>
        <h1>视频加载失败</h1>
        <p>{message}</p>
        <a href={`/video/${FALLBACK_BVID}`}>回到本地测试视频</a>
      </section>
    </main>
  )
}

function Metric({ label, value }: { label: string; value: MetricValue }) {
  return (
    <div>
      <strong>{formatCount(value)}</strong>
      <span>{label}</span>
    </div>
  )
}

function AuthMenu() {
  const [open, setOpen] = useState(false)
  const [session, setSession] = useState<AuthSession | null>(readAuthSession)
  const [username, setUsername] = useState('demo')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const authRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!open) return

    function closeOnOutsideClick(event: PointerEvent) {
      if (!authRef.current?.contains(event.target as Node)) {
        setOpen(false)
      }
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setOpen(false)
      }
    }

    document.addEventListener('pointerdown', closeOnOutsideClick)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsideClick)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  async function login(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setPending(true)
    setError('')

    try {
      const response = await postJson<unknown>('/api/v1/auth/login', {
        username,
        password,
      })
      const nextSession = normalizeAuthSession(response)
      window.localStorage.setItem(
        AUTH_STORAGE_KEY,
        JSON.stringify(nextSession),
      )
      setSession(nextSession)
      setPassword('')
    } catch (loginError) {
      const message = toErrorMessage(loginError, '')
      setError(
        message.startsWith('401')
          ? '用户名或密码不正确'
          : '登录失败，请稍后重试',
      )
    } finally {
      setPending(false)
    }
  }

  async function logout() {
    if (!session) return
    setPending(true)
    setError('')

    try {
      await postJson('/api/v1/auth/logout', {}, session.accessToken)
    } catch (logoutError) {
      const message = toErrorMessage(logoutError, '')
      if (!message.includes('401')) {
        setError('服务端会话暂未撤销，本机已退出')
      }
    } finally {
      window.localStorage.removeItem(AUTH_STORAGE_KEY)
      setSession(null)
      setPending(false)
    }
  }

  const userLabel = session?.user.displayName || session?.user.username || '用户'

  return (
    <div className="auth-menu" ref={authRef}>
      <button
        type="button"
        className={session ? 'auth-trigger signed-in' : 'auth-trigger'}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls="auth-popover"
        onClick={() => {
          setOpen((value) => !value)
          setError('')
        }}
      >
        {session ? userLabel.slice(0, 1).toUpperCase() : '登录'}
      </button>

      {open && (
        <section
          id="auth-popover"
          className="auth-popover"
          role="dialog"
          aria-labelledby="auth-popover-title"
        >
          {session ? (
            <div className="auth-account">
              <div className="auth-account-head">
                <div className="auth-avatar" aria-hidden="true">
                  {session.user.avatarUrl ? (
                    <img src={session.user.avatarUrl} alt="" />
                  ) : (
                    userLabel.slice(0, 1).toUpperCase()
                  )}
                </div>
                <div>
                  <strong id="auth-popover-title">{userLabel}</strong>
                  <span>@{session.user.username}</span>
                </div>
              </div>
              {session.user.bio && <p>{session.user.bio}</p>}
              {error && (
                <div className="auth-error" role="status">
                  {error}
                </div>
              )}
              <button
                type="button"
                className="auth-secondary-button"
                disabled={pending}
                onClick={() => void logout()}
              >
                {pending ? '正在退出…' : '退出登录'}
              </button>
            </div>
          ) : (
            <form className="auth-form" onSubmit={login}>
              <div className="auth-form-heading">
                <strong id="auth-popover-title">登录</strong>
                <span>继续使用 bilibili-lite</span>
              </div>
              <label>
                <span>用户名</span>
                <input
                  name="username"
                  autoComplete="username"
                  value={username}
                  onChange={(event) => {
                    setUsername(event.target.value)
                    setError('')
                  }}
                  required
                />
              </label>
              <label>
                <span>密码</span>
                <input
                  name="password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(event) => {
                    setPassword(event.target.value)
                    setError('')
                  }}
                  required
                  autoFocus
                />
              </label>
              {error && (
                <div className="auth-error" role="alert">
                  {error}
                </div>
              )}
              <button
                type="submit"
                className="auth-primary-button"
                disabled={pending}
              >
                {pending ? '登录中…' : '登录'}
              </button>
            </form>
          )}
        </section>
      )}
    </div>
  )
}

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}: ${url}`)
  }
  return (await response.json()) as T
}

async function postJson<T = unknown>(
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

function getBvidFromPath() {
  const match = window.location.pathname.match(/^\/video\/([^/]+)$/)
  return match?.[1] ?? FALLBACK_BVID
}

function toNumber(value: MetricValue | undefined) {
  if (typeof value === 'number') return value
  if (!value) return 0
  return Number(value)
}

function normalizeVideoDetail(value: unknown): VideoDetail {
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
  }
}

function normalizeVideoPlay(value: unknown): VideoPlay {
  const record = asRecord(value)
  const danmaku = asRecord(record.danmaku)

  return {
    bvid: readString(record, 'bvid'),
    streams: readArray(record, 'streams').map((stream) => {
      const item = asRecord(stream)
      return {
        id: readString(item, 'id'),
        label: readString(item, 'label'),
        codec: readString(item, 'codec'),
        mimeType: readString(item, 'mimeType', 'mime_type'),
        url: readString(item, 'url'),
        width: Number(readMetric(item, 'width')),
        height: Number(readMetric(item, 'height')),
        bandwidth: Number(readMetric(item, 'bandwidth')),
        defaultStream: readBoolean(item, 'defaultStream', 'default_stream'),
      }
    }),
    danmaku: {
      enabled: readBoolean(danmaku, 'enabled'),
      format: readString(danmaku, 'format'),
      items: readArray(danmaku, 'items').map((danmakuItem) => {
        const item = asRecord(danmakuItem)
        return {
          timeSeconds: Number(readMetric(item, 'timeSeconds', 'time_seconds')),
          text: readString(item, 'text'),
          color: readString(item, 'color') || '#ffffff',
        }
      }),
    },
  }
}

function normalizeAuthSession(value: unknown): AuthSession {
  const record = asRecord(value)
  const user = asRecord(record.user)
  const expiresAt = readTimestamp(record, 'expiresAt', 'expires_at')
  const accessToken = readString(record, 'accessToken', 'access_token')

  if (!accessToken || !expiresAt || !readString(user, 'username')) {
    throw new Error('登录响应不完整')
  }

  return {
    accessToken,
    expiresAt,
    user: {
      id: Number(readMetric(user, 'id')),
      username: readString(user, 'username'),
      displayName: readString(user, 'displayName', 'display_name'),
      avatarUrl: readString(user, 'avatarUrl', 'avatar_url'),
      bio: readString(user, 'bio'),
    },
  }
}

function readAuthSession(): AuthSession | null {
  try {
    const stored = window.localStorage.getItem(AUTH_STORAGE_KEY)
    if (!stored) return null
    const session = normalizeAuthSession(JSON.parse(stored))
    if (new Date(session.expiresAt).getTime() <= Date.now()) {
      window.localStorage.removeItem(AUTH_STORAGE_KEY)
      return null
    }
    return session
  } catch {
    window.localStorage.removeItem(AUTH_STORAGE_KEY)
    return null
  }
}

function toErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback
}

function asRecord(value: unknown): Record<string, unknown> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return {}
}

function readString(record: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string') return value
  }
  return ''
}

function readMetric(
  record: Record<string, unknown>,
  ...keys: string[]
): MetricValue {
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

function formatCount(value: MetricValue | undefined) {
  const count = toNumber(value)
  if (count >= 10000) return `${(count / 10000).toFixed(1)}万`
  return new Intl.NumberFormat('zh-CN').format(count)
}

function formatBitrate(value: number) {
  if (!value) return '-'
  return `${(value / 1000 / 1000).toFixed(1)} Mbps`
}

function formatDate(value?: string) {
  if (!value) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

export default App
