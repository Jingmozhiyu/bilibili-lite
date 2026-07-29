import type { MediaPlayerClass } from 'dashjs'
import { Check, ChevronDown, ChevronUp, CircleAlert, LogIn, Play, ShieldCheck, Trash2, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { authorizedFetch, authorizedJson, normalizeAuthSession, normalizeVideoDetail, normalizeVideoList, normalizeVideoPlay, postJson, toErrorMessage } from '../api'
import { useAuth } from '../auth/useAuth'
import type { AuthSession, VideoDetail, VideoPlay } from '../types'
import { formatDate, formatDuration } from '../utils/format'

type AdminVideoView = 'pending_review' | 'rejected'

export function AdminReviewPage() {
  const { session, restoring, setSession } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const view: AdminVideoView = searchParams.get('status') === 'rejected' ? 'rejected' : 'pending_review'
  const [videos, setVideos] = useState<VideoDetail[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (restoring) return
    if (!session?.user.isAdmin) return
    let active = true
    void loadAdminVideos(session, view, '').then((result) => {
      if (!active) return
      setError('')
      if (result.session.accessToken !== session.accessToken) setSession(result.session)
      setVideos(result.videos)
      setNextPageToken(result.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, view === 'rejected' ? '已下架视频加载失败' : '审核队列加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [restoring, session, setSession, view])

  async function loadMore() {
    if (!session || !nextPageToken) return
    try {
      const result = await loadAdminVideos(session, view, nextPageToken)
      setSession(result.session)
      setVideos((current) => [...current, ...result.videos])
      setNextPageToken(result.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页加载失败'))
    }
  }

  function selectView(nextView: AdminVideoView) {
    if (nextView === view) return
    setLoading(true)
    setError('')
    setVideos([])
    setNextPageToken('')
    setSearchParams(nextView === 'rejected' ? { status: 'rejected' } : {})
  }

  if (restoring) {
    return <main className="admin-page"><div className="admin-list">{Array.from({ length: 4 }, (_, index) => <div className="admin-review-skeleton" key={index} />)}</div></main>
  }
  if (!session?.user.isAdmin) {
    return <AdminAccessPanel />
  }
  if (loading) {
    return <main className="admin-page"><div className="admin-list">{Array.from({ length: 4 }, (_, index) => <div className="admin-review-skeleton" key={index} />)}</div></main>
  }

  return (
    <main className="admin-page">
      <header className="admin-heading">
        <div>
          <h1>内容管理</h1>
          <p>{view === 'rejected' ? '复核已下架内容，必要时永久清理本地媒体文件。' : '播放投稿内容，确认后公开或填写驳回原因。'}</p>
        </div>
        <div className="admin-heading-meta">
          <span><ShieldCheck size={15} />{session.user.displayName}</span>
          <strong>{videos.length} 条{view === 'rejected' ? '已下架' : '待审'}</strong>
        </div>
      </header>
      <nav className="admin-status-tabs" aria-label="内容状态">
        <button type="button" className={view === 'pending_review' ? 'active' : ''} onClick={() => selectView('pending_review')}>待审核</button>
        <button type="button" className={view === 'rejected' ? 'active' : ''} onClick={() => selectView('rejected')}>已下架</button>
      </nav>
      {error && <p className="inline-error" role="status">{error}</p>}
      {videos.length === 0 ? (
        <div className="admin-empty">
          <ShieldCheck size={34} />
          <h2>{view === 'rejected' ? '没有已下架视频' : '待审队列为空'}</h2>
          <p>{view === 'rejected' ? '仅下架但仍保留媒体的视频会出现在这里。' : '新投稿提交审核后会出现在这里。'}</p>
        </div>
      ) : (
        <div className="admin-list">
          {videos.map((video) => view === 'rejected'
            ? <RejectedItem key={video.bvid} video={video} onDeleted={() => setVideos((current) => current.filter((item) => item.bvid !== video.bvid))} />
            : <ReviewItem key={video.bvid} video={video} onResolved={() => setVideos((current) => current.filter((item) => item.bvid !== video.bvid))} />)}
        </div>
      )}
      {nextPageToken && <button className="load-more-button" type="button" onClick={() => void loadMore()}>加载更多</button>}
    </main>
  )
}

function AdminAccessPanel() {
  const { session, setSession } = useAuth()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  async function login(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setPending(true)
    setError('')
    try {
      const response = await postJson<unknown>('/api/v1/auth/login', { username, password })
      const nextSession = normalizeAuthSession(response)
      if (!nextSession.user.isAdmin) {
        setError('该账号没有管理员权限')
        return
      }
      setSession(nextSession)
      setPassword('')
    } catch (loginError) {
      const message = toErrorMessage(loginError, '')
      setError(message.startsWith('401') ? '管理员账号或密码不正确' : '登录失败，请稍后重试')
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="admin-page admin-access-page">
      <section className="admin-access-panel" aria-labelledby="admin-access-title">
        <span className="admin-access-icon"><ShieldCheck size={25} /></span>
        <div>
          <h1 id="admin-access-title">管理员登录</h1>
          <p>验证管理员身份后进入投稿审核台。</p>
        </div>
        {session && <p className="admin-current-user">当前账号 @{session.user.username} 没有管理员权限，可以在下方切换账号。</p>}
        <form className="admin-login-form" onSubmit={login}>
          <label><span>管理员账号</span><input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /></label>
          <label><span>密码</span><input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required autoFocus /></label>
          {error && <p className="form-error" role="alert">{error}</p>}
          <button type="submit" disabled={pending}><LogIn size={17} />{pending ? '正在验证' : '进入审核台'}</button>
        </form>
        <small>本地管理员账号由启动 seed 初始化。</small>
      </section>
    </main>
  )
}

function ReviewItem({ video, onResolved }: { video: VideoDetail; onResolved: () => void }) {
  const { session, setSession } = useAuth()
  const [expanded, setExpanded] = useState(false)
  const [play, setPlay] = useState<VideoPlay | null>(null)
  const [reason, setReason] = useState('')
  const [pending, setPending] = useState<'approve' | 'reject' | ''>('')
  const [error, setError] = useState('')

  async function openPreview() {
    setExpanded((value) => !value)
    if (play || !session) return
    try {
      const result = await authorizedJson<unknown>(`/api/v1/admin/videos/${encodeURIComponent(video.bvid)}/play`, {}, session)
      setSession(result.session)
      setPlay(normalizeVideoPlay(result.data))
    } catch (loadError) {
      setError(toErrorMessage(loadError, '预览加载失败'))
    }
  }

  async function decide(action: 'approve' | 'reject') {
    if (!session || (action === 'reject' && !reason.trim())) return
    setPending(action)
    setError('')
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/admin/videos/${encodeURIComponent(video.bvid)}/${action}`,
        {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(action === 'reject' ? { reason } : {}),
        },
        session,
      )
      setSession(result.session)
      normalizeVideoDetail(result.data)
      onResolved()
    } catch (decisionError) {
      setError(toErrorMessage(decisionError, action === 'approve' ? '通过失败' : '驳回失败'))
    } finally {
      setPending('')
    }
  }

  return (
    <article className="admin-review-item">
      <button type="button" className="review-cover" onClick={() => void openPreview()} aria-label={`预览 ${video.title}`}>
        {video.coverUrl ? <img src={video.coverUrl} alt="" /> : <span><Play size={24} /></span>}
        <em>{formatDuration(video.durationSeconds)}</em>
      </button>
      <div className="review-copy">
        <div className="review-title-row">
          <div><h2>{video.title}</h2><p>{video.bvid} · {video.ownerName} · 提交于 {formatDate(video.submittedAt)}</p></div>
          <button type="button" className="review-expand" onClick={() => void openPreview()}>{expanded ? <ChevronUp size={18} /> : <ChevronDown size={18} />}{expanded ? '收起预览' : '播放预览'}</button>
        </div>
        {video.description && <p className="review-description">{video.description}</p>}
        {video.tags.length > 0 && <div className="review-tags">{video.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>}
        {expanded && <ReviewPreview play={play} />}
        <div className="review-actions">
          <label><span>驳回原因</span><input value={reason} maxLength={1000} onChange={(event) => setReason(event.target.value)} placeholder="驳回时必填，投稿者可见" /></label>
          <button type="button" className="review-reject" disabled={!reason.trim() || Boolean(pending)} onClick={() => void decide('reject')}><X size={17} />{pending === 'reject' ? '处理中' : '驳回'}</button>
          <button type="button" className="review-approve" disabled={Boolean(pending)} onClick={() => void decide('approve')}><Check size={17} />{pending === 'approve' ? '处理中' : '通过并公开'}</button>
        </div>
        {error && <p className="inline-error"><CircleAlert size={15} />{error}</p>}
      </div>
    </article>
  )
}

function RejectedItem({ video, onDeleted }: { video: VideoDetail; onDeleted: () => void }) {
  const { session, setSession } = useAuth()
  const [expanded, setExpanded] = useState(false)
  const [play, setPlay] = useState<VideoPlay | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  async function openPreview() {
    setExpanded((value) => !value)
    if (play || !session) return
    try {
      const result = await authorizedJson<unknown>(`/api/v1/admin/videos/${encodeURIComponent(video.bvid)}/play`, {}, session)
      setSession(result.session)
      setPlay(normalizeVideoPlay(result.data))
    } catch (loadError) {
      setError(toErrorMessage(loadError, '预览加载失败'))
    }
  }

  async function deleteMedia() {
    if (!session || pending) return
    if (!window.confirm(`永久删除 ${video.bvid} 的 DASH 分片和封面？此操作不可恢复。`)) return
    setPending(true)
    setError('')
    try {
      const reason = video.reviewReason?.trim() || '管理员清理已下架视频文件'
      const query = new URLSearchParams({ reason })
      const result = await authorizedFetch(
        `/api/v1/admin/videos/${encodeURIComponent(video.bvid)}?${query}`,
        { method: 'DELETE' },
        session,
      )
      setSession(result.session)
      onDeleted()
    } catch (deleteError) {
      setError(toErrorMessage(deleteError, '媒体文件删除失败'))
    } finally {
      setPending(false)
    }
  }

  return (
    <article className="admin-review-item">
      <button type="button" className="review-cover" onClick={() => void openPreview()} aria-label={`预览 ${video.title}`}>
        {video.coverUrl ? <img src={video.coverUrl} alt="" /> : <span><Play size={24} /></span>}
        <em>{formatDuration(video.durationSeconds)}</em>
      </button>
      <div className="review-copy">
        <div className="review-title-row">
          <div>
            <h2><Link to={`/video/${video.bvid}`}>{video.title}</Link></h2>
            <p>{video.bvid} · {video.ownerName} · 下架于 {formatDate(video.reviewedAt)}</p>
          </div>
          <button type="button" className="review-expand" onClick={() => void openPreview()}>{expanded ? <ChevronUp size={18} /> : <ChevronDown size={18} />}{expanded ? '收起预览' : '播放预览'}</button>
        </div>
        <p className="review-moderation-reason"><strong>下架原因</strong>{video.reviewReason || '未记录原因'}</p>
        {expanded && <ReviewPreview play={play} />}
        <div className="review-cleanup-actions">
          <span>当前仅对公众隐藏，DASH 分片仍保留。</span>
          <button type="button" className="review-delete" disabled={pending} onClick={() => void deleteMedia()}>
            <Trash2 size={17} />{pending ? '正在删除' : '永久删除文件'}
          </button>
        </div>
        {error && <p className="inline-error"><CircleAlert size={15} />{error}</p>}
      </div>
    </article>
  )
}

function ReviewPreview({ play }: { play: VideoPlay | null }) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const playerRef = useRef<MediaPlayerClass | null>(null)
  const stream = play?.streams[0]

  useEffect(() => {
    const video = videoRef.current
    if (!video || !stream) return
    let cancelled = false
    void import('dashjs').then(({ Debug, MediaPlayer }) => {
      if (cancelled) return
      const player = MediaPlayer().create()
      playerRef.current = player
      player.updateSettings({ debug: { logLevel: Debug.LOG_LEVEL_FATAL } })
      player.initialize(video, stream.url, false)
    })
    return () => {
      cancelled = true
      playerRef.current?.destroy()
      playerRef.current = null
    }
  }, [stream])

  if (!play) return <div className="review-preview-loading">正在加载 DASH 预览…</div>
  if (!stream) return <div className="review-preview-loading">没有可用的播放流</div>
  return <div className="review-preview"><video ref={videoRef} controls playsInline /></div>
}

async function loadAdminVideos(session: AuthSession, status: AdminVideoView, pageToken: string) {
  const query = new URLSearchParams({ status, page_size: '10' })
  if (pageToken) query.set('page_token', pageToken)
  const result = await authorizedJson<unknown>(`/api/v1/admin/videos?${query}`, {}, session)
  const page = normalizeVideoList(result.data)
  return { ...page, session: result.session }
}
