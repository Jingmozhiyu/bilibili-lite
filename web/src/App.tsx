import type { MediaPlayerClass } from 'dashjs'
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
  refreshToken: string
  expiresAt: string
  refreshExpiresAt: string
  user: AuthUser
}

type VideoLike = {
  bvid: string
  liked: boolean
  likeCount: MetricValue
}

type UploadResult = {
  bvid: string
  manifestUrl: string
  videoUrl: string
}

const AUTH_STORAGE_KEY = 'bilibili-lite.auth-session'

function App() {
  const bvid = getBvidFromPath()
  return bvid ? <VideoPage bvid={bvid} /> : <HomePage />
}

function VideoPage({ bvid }: { bvid: string }) {
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [selectedStreamId, setSelectedStreamId] = useState<string>('')
  const [danmakuOn, setDanmakuOn] = useState(true)
  const [currentTime, setCurrentTime] = useState(0)
  const [session, setSession] = useState<AuthSession | null>(readAuthSession)
  const [authOpen, setAuthOpen] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [like, setLike] = useState({ liked: false, count: 0, pending: false })
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const dashPlayerRef = useRef<MediaPlayerClass | null>(null)

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
        setLike({ liked: false, count: toNumber(detail.likeCount), pending: false })
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

  useEffect(() => {
    if (state.status !== 'ready') return
    if (!session) return

    let ignore = false
    authorizedFetch(`/api/v1/videos/${encodeURIComponent(bvid)}/like`, {}, session)
      .then(({ response, session: nextSession }) => {
        if (ignore) return
        persistAuthSession(nextSession)
        setSession(nextSession)
        return parseJsonResponse<unknown>(response)
      })
      .then((value) => {
        if (ignore || value === undefined) return
        const nextLike = normalizeVideoLike(value)
        setLike({ liked: nextLike.liked, count: toNumber(nextLike.likeCount), pending: false })
      })
      .catch(() => {
        if (!ignore) setLike((value) => ({ ...value, liked: false, pending: false }))
      })
    return () => {
      ignore = true
    }
  }, [bvid, session, state])

  useEffect(() => {
    const video = videoRef.current
    if (!video || !selectedStream) return
    const resumeAt = video.currentTime
    const shouldPlay = !video.paused || resumeAt === 0
    let cancelled = false

    dashPlayerRef.current?.destroy()
    dashPlayerRef.current = null

    void import('dashjs').then(({ Debug, MediaPlayer }) => {
      if (cancelled) return
      const player = MediaPlayer().create()
      dashPlayerRef.current = player
      player.updateSettings({
        debug: {
          logLevel: Debug.LOG_LEVEL_FATAL,
        },
        streaming: {
          cmcd: {
            enabled: false,
            applyParametersFromMpd: false,
            eventTargets: [],
          },
        },
      })
      player.initialize(video, selectedStream.url, shouldPlay)
      if (resumeAt > 0) player.seek(resumeAt)
    })

    return () => {
      cancelled = true
      dashPlayerRef.current?.destroy()
      dashPlayerRef.current = null
      video.removeAttribute('src')
      video.load()
    }
  }, [selectedStream])

  async function toggleLike() {
    if (!session) {
      setAuthOpen(true)
      return
    }
    const desired = !like.liked
    setLike((value) => ({ ...value, pending: true }))
    try {
      const { response, session: nextSession } = await authorizedFetch(
        `/api/v1/videos/${encodeURIComponent(bvid)}/like`,
        { method: desired ? 'POST' : 'DELETE' },
        session,
      )
      const nextLike = normalizeVideoLike(await parseJsonResponse(response))
      persistAuthSession(nextSession)
      setSession(nextSession)
      setLike({ liked: nextLike.liked, count: toNumber(nextLike.likeCount), pending: false })
    } catch {
      setLike((value) => ({ ...value, pending: false }))
    }
  }

  function changeSession(nextSession: AuthSession | null) {
    setSession(nextSession)
    if (!nextSession && state.status === 'ready') {
      setLike({ liked: false, count: toNumber(state.detail.likeCount), pending: false })
    }
  }

  function switchStream(streamId: string) {
    setSelectedStreamId(streamId)
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
        <a className="brand" href="/">
          <span className="brand-mark">b</span>
          <span>bilibili-lite</span>
        </a>
        <div className="search-box" role="search">
          <input aria-label="搜索视频" placeholder="搜索视频、番剧、直播" />
          <button type="button">搜索</button>
        </div>
        <div className="topbar-actions">
          <nav className="quick-links" aria-label="快捷入口">
            <a href="/">首页</a>
            <a href={`/video/${detail.bvid}`}>动态</a>
            <a href={`/video/${detail.bvid}`}>收藏</a>
          </nav>
          <AuthMenu
            session={session}
            onSessionChange={changeSession}
            open={authOpen}
            onOpenChange={setAuthOpen}
            uploadOpen={uploadOpen}
            onUploadOpenChange={setUploadOpen}
          />
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
            <button
              type="button"
              className={like.liked ? 'metric-action active' : 'metric-action'}
              aria-pressed={like.liked}
              disabled={like.pending}
              onClick={() => void toggleLike()}
            >
              <strong>{formatCount(like.count)}</strong>
              <span>{like.pending ? '处理中' : like.liked ? '已点赞' : '点赞'}</span>
            </button>
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

function HomePage() {
  const [session, setSession] = useState<AuthSession | null>(readAuthSession)
  const [authOpen, setAuthOpen] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const userLabel = session?.user.displayName || session?.user.username

  function startUpload() {
    setUploadOpen(true)
    setAuthOpen(true)
  }

  return (
    <main className="page-shell home-page">
      <header className="topbar home-topbar" aria-label="主导航">
        <a className="brand" href="/">
          <span className="brand-mark">b</span>
          <span>bilibili-lite</span>
        </a>
        <div className="topbar-actions">
          <AuthMenu
            session={session}
            onSessionChange={setSession}
            open={authOpen}
            onOpenChange={setAuthOpen}
            uploadOpen={uploadOpen}
            onUploadOpenChange={setUploadOpen}
          />
        </div>
      </header>

      <section className="home-empty" aria-labelledby="home-empty-title">
        <div className="home-empty-content">
          <div className="home-upload-mark" aria-hidden="true">
            ↑
          </div>
          <h1 id="home-empty-title">还没有视频</h1>
          <p>
            {session
              ? `${userLabel}，上传第一支视频开始放映。`
              : '登录后上传第一支视频，发布后从 BV1 开始。'}
          </p>
          <button type="button" className="home-upload-button" onClick={startUpload}>
            {session ? '上传第一支视频' : '登录并上传'}
          </button>
        </div>
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
        <a href="/">返回首页</a>
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

function AuthMenu({
  session,
  onSessionChange,
  open,
  onOpenChange,
  uploadOpen,
  onUploadOpenChange,
}: {
  session: AuthSession | null
  onSessionChange: (session: AuthSession | null) => void
  open: boolean
  onOpenChange: (open: boolean) => void
  uploadOpen: boolean
  onUploadOpenChange: (open: boolean) => void
}) {
  const [username, setUsername] = useState('demo')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [uploadTitle, setUploadTitle] = useState('')
  const [uploadDescription, setUploadDescription] = useState('')
  const [uploadTags, setUploadTags] = useState('')
  const [uploadFile, setUploadFile] = useState<File | null>(null)
  const [uploadPhase, setUploadPhase] = useState<'idle' | 'uploading' | 'processing' | 'success' | 'error'>('idle')
  const [uploadProgress, setUploadProgress] = useState(0)
  const [uploadMessage, setUploadMessage] = useState('')
  const [uploadResult, setUploadResult] = useState<UploadResult | null>(null)
  const authRef = useRef<HTMLDivElement | null>(null)
  const uploadRequestRef = useRef<XMLHttpRequest | null>(null)

  useEffect(() => () => uploadRequestRef.current?.abort(), [])

  useEffect(() => {
    if (!open) return

    function closeOnOutsideClick(event: PointerEvent) {
      if (!authRef.current?.contains(event.target as Node)) {
        onOpenChange(false)
      }
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onOpenChange(false)
      }
    }

    document.addEventListener('pointerdown', closeOnOutsideClick)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsideClick)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [onOpenChange, open])

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
      persistAuthSession(nextSession)
      onSessionChange(nextSession)
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
      onSessionChange(null)
      onUploadOpenChange(false)
      setPending(false)
    }
  }

  async function uploadVideo(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!session || !uploadFile) return

    setUploadPhase('uploading')
    setUploadProgress(0)
    setUploadMessage('')
    setUploadResult(null)

    try {
      const activeSession = await ensureFreshAuthSession(session)
      persistAuthSession(activeSession)
      onSessionChange(activeSession)

      const form = new FormData()
      form.append('title', uploadTitle)
      form.append('description', uploadDescription)
      form.append('tags', uploadTags)
      form.append('file', uploadFile)

      const result = await new Promise<UploadResult>((resolve, reject) => {
        const request = new XMLHttpRequest()
        uploadRequestRef.current = request
        request.open('POST', '/api/v1/videos/upload')
        request.setRequestHeader('Authorization', `Bearer ${activeSession.accessToken}`)
        request.upload.addEventListener('progress', (progressEvent) => {
          if (!progressEvent.lengthComputable) return
          const percent = Math.round((progressEvent.loaded / progressEvent.total) * 100)
          setUploadProgress(percent)
          if (percent >= 100) setUploadPhase('processing')
        })
        request.addEventListener('load', () => {
          uploadRequestRef.current = null
          const payload = asRecord(parseJSON(request.responseText))
          if (request.status < 200 || request.status >= 300) {
            reject(new Error(readString(payload, 'message') || `上传失败（${request.status}）`))
            return
          }
          resolve({
            bvid: readString(payload, 'bvid'),
            manifestUrl: readString(payload, 'manifestUrl', 'manifest_url'),
            videoUrl: readString(payload, 'videoUrl', 'video_url'),
          })
        })
        request.addEventListener('error', () => reject(new Error('网络连接中断')))
        request.addEventListener('abort', () => reject(new Error('上传已取消')))
        request.send(form)
      })

      setUploadResult(result)
      setUploadPhase('success')
      setUploadMessage('视频已完成 DASH 处理并发布')
    } catch (uploadError) {
      setUploadPhase('error')
      setUploadMessage(toErrorMessage(uploadError, '上传失败'))
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
          onOpenChange(!open)
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
              <div className="auth-account-actions">
                <button
                  type="button"
                  className="auth-primary-button"
                  disabled={pending}
                  onClick={() => onUploadOpenChange(!uploadOpen)}
                  aria-expanded={uploadOpen}
                  aria-controls="upload-popover"
                >
                  投稿视频
                </button>
                <button
                  type="button"
                  className="auth-secondary-button"
                  disabled={pending || uploadPhase === 'uploading' || uploadPhase === 'processing'}
                  onClick={() => void logout()}
                >
                  {pending ? '正在退出…' : '退出登录'}
                </button>
              </div>
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

      {open && session && uploadOpen && (
        <section id="upload-popover" className="upload-popover" role="dialog" aria-labelledby="upload-title">
          <form className="upload-form" onSubmit={uploadVideo}>
            <div className="upload-heading">
              <div>
                <strong id="upload-title">投稿视频</strong>
                <span>MP4 将转为 DASH 音视频分片</span>
              </div>
              <button
                type="button"
                className="upload-close"
                aria-label="关闭投稿面板"
                onClick={() => onUploadOpenChange(false)}
              >
                ×
              </button>
            </div>

            <label className="upload-file-field">
              <span>{uploadFile ? uploadFile.name : '选择 MP4 文件'}</span>
              <small>{uploadFile ? formatFileSize(uploadFile.size) : '单文件，最大 2 GB'}</small>
              <input
                type="file"
                accept="video/mp4,.mp4"
                required
                onChange={(event) => {
                  setUploadFile(event.target.files?.[0] ?? null)
                  setUploadPhase('idle')
                  setUploadMessage('')
                }}
              />
            </label>
            <label>
              <span>标题</span>
              <input maxLength={200} value={uploadTitle} onChange={(event) => setUploadTitle(event.target.value)} required />
            </label>
            <label>
              <span>简介</span>
              <textarea maxLength={10000} rows={4} value={uploadDescription} onChange={(event) => setUploadDescription(event.target.value)} />
            </label>
            <label>
              <span>标签</span>
              <input value={uploadTags} onChange={(event) => setUploadTags(event.target.value)} placeholder="多个标签用逗号分隔" />
            </label>

            {uploadPhase !== 'idle' && (
              <div className="upload-status" aria-live="polite">
                <div className="upload-status-row">
                  <span>{uploadPhase === 'uploading' ? '正在上传' : uploadPhase === 'processing' ? '正在生成 DASH 分片' : uploadPhase === 'success' ? '处理完成' : '处理失败'}</span>
                  {uploadPhase === 'uploading' && <strong>{uploadProgress}%</strong>}
                </div>
                <div className={uploadPhase === 'processing' ? 'upload-progress processing' : 'upload-progress'}>
                  <span style={{ width: uploadPhase === 'processing' ? '38%' : `${uploadProgress}%` }} />
                </div>
                {uploadMessage && <p className={uploadPhase === 'error' ? 'upload-message error' : 'upload-message'}>{uploadMessage}</p>}
                {uploadResult && <a className="upload-result-link" href={uploadResult.videoUrl}>查看 {uploadResult.bvid}</a>}
              </div>
            )}

            <button className="auth-primary-button" type="submit" disabled={!uploadFile || !uploadTitle.trim() || uploadPhase === 'uploading' || uploadPhase === 'processing'}>
              {uploadPhase === 'uploading' ? '上传中…' : uploadPhase === 'processing' ? '处理中…' : '开始上传'}
            </button>
          </form>
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

async function authorizedFetch(
  url: string,
  init: RequestInit,
  session: AuthSession,
): Promise<{ response: Response; session: AuthSession }> {
  let activeSession = await ensureFreshAuthSession(session)
  let response = await fetch(url, {
    ...init,
    headers: {
      ...init.headers,
      Authorization: `Bearer ${activeSession.accessToken}`,
    },
  })
  if (response.status === 401) {
    activeSession = await refreshAuthSession(activeSession)
    response = await fetch(url, {
      ...init,
      headers: {
        ...init.headers,
        Authorization: `Bearer ${activeSession.accessToken}`,
      },
    })
  }
  if (!response.ok) {
    const payload = asRecord(await response.json().catch(() => null))
    throw new Error(readString(payload, 'message') || `${response.status} ${response.statusText}`)
  }
  return { response, session: activeSession }
}

async function ensureFreshAuthSession(session: AuthSession) {
  const refreshBefore = new Date(session.expiresAt).getTime() - 60_000
  if (Date.now() < refreshBefore) return session
  return refreshAuthSession(session)
}

async function refreshAuthSession(session: AuthSession) {
  const response = await postJson<unknown>('/api/v1/auth/refresh', {
    refreshToken: session.refreshToken,
  })
  return normalizeAuthSession(response)
}

async function parseJsonResponse<T>(response: Response): Promise<T> {
  return (await response.json()) as T
}

function persistAuthSession(session: AuthSession) {
  window.localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(session))
}

function parseJSON(value: string): unknown {
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

function getBvidFromPath() {
  const match = window.location.pathname.match(/^\/video\/([^/]+)$/)
  return match?.[1] ?? null
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
  const streams = readArray(record, 'streams')
    .map((stream) => {
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
    })
    .filter(
      (stream) =>
        stream.mimeType === 'application/dash+xml' && stream.url.endsWith('.mpd'),
    )

  return {
    bvid: readString(record, 'bvid'),
    streams,
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
  const refreshExpiresAt = readTimestamp(record, 'refreshExpiresAt', 'refresh_expires_at')
  const accessToken = readString(record, 'accessToken', 'access_token')
  const refreshToken = readString(record, 'refreshToken', 'refresh_token')

  if (!accessToken || !refreshToken || !expiresAt || !refreshExpiresAt || !readString(user, 'username')) {
    throw new Error('登录响应不完整')
  }

  return {
    accessToken,
    refreshToken,
    expiresAt,
    refreshExpiresAt,
    user: {
      id: Number(readMetric(user, 'id')),
      username: readString(user, 'username'),
      displayName: readString(user, 'displayName', 'display_name'),
      avatarUrl: readString(user, 'avatarUrl', 'avatar_url'),
      bio: readString(user, 'bio'),
    },
  }
}

function normalizeVideoLike(value: unknown): VideoLike {
  const record = asRecord(value)
  return {
    bvid: readString(record, 'bvid'),
    liked: readBoolean(record, 'liked'),
    likeCount: readMetric(record, 'likeCount', 'like_count'),
  }
}

function readAuthSession(): AuthSession | null {
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

function formatFileSize(bytes: number) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(2)} GB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

export default App
