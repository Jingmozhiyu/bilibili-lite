import type { MediaPlayerClass } from 'dashjs'
import { AlertCircle, ChevronLeft, ShieldAlert, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  authorizedFetch,
  authorizedJson,
  fetchJson,
  normalizeDanmaku,
  normalizeVideoDetail,
  normalizeVideoEngagement,
  normalizeVideoLike,
  normalizeVideoList,
  normalizeVideoPlay,
  normalizeVideoViewResult,
  normalizeVideoViewSession,
  toErrorMessage,
  toNumber,
} from '../api'
import { useAuth } from '../auth/useAuth'
import { CommentSection } from '../components/CommentSection'
import { DanmakuComposer } from '../components/DanmakuComposer'
import { DanmakuPanel } from '../components/DanmakuPanel'
import { InteractionBar } from '../components/InteractionBar'
import { RelatedVideos } from '../components/RelatedVideos'
import { BiliDanmakuIcon, BiliViewIcon } from '../components/BiliIcons'
import { BiliVideoPlayer } from '../components/BiliVideoPlayer'
import type { DanmakuItem, VideoDetail, VideoEngagement, VideoPlay } from '../types'
import { formatCount, formatDate } from '../utils/format'

type LoadState =
  | { status: 'loading' }
  | { status: 'ready'; detail: VideoDetail; play: VideoPlay; adminPreview: boolean }
  | { status: 'error'; message: string }

export function VideoPage() {
  const { bvid = '' } = useParams()
  return <VideoContent key={bvid} bvid={bvid} />
}

function VideoContent({ bvid }: { bvid: string }) {
  const { session, restoring, setSession } = useAuth()
  const navigate = useNavigate()
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const adminPreviewMode = state.status === 'ready' && state.adminPreview
  const [selectedStreamId, setSelectedStreamId] = useState('')
  const [danmakuOn, setDanmakuOn] = useState(true)
  const [currentTime, setCurrentTime] = useState(0)
  const [engagement, setEngagement] = useState<VideoEngagement | null>(null)
  const [interactionPending, setInteractionPending] = useState('')
  const [interactionMessage, setInteractionMessage] = useState('')
  const [danmakuPending, setDanmakuPending] = useState(false)
  const [takeDownReason, setTakeDownReason] = useState('')
  const [moderationPending, setModerationPending] = useState<'take-down' | 'delete' | ''>('')
  const [relatedVideos, setRelatedVideos] = useState<VideoDetail[]>([])
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const dashPlayerRef = useRef<MediaPlayerClass | null>(null)
  const viewSessionRef = useRef('')
  const watchedSecondsRef = useRef(0)
  const previousPlaybackTimeRef = useRef<number | null>(null)
  const startingViewRef = useRef(false)
  const completingViewRef = useRef(false)
  const completedViewRef = useRef(false)

  useEffect(() => {
    if (restoring) return
    let active = true
    async function loadVideo() {
      try {
        const [detailPayload, playPayload] = await Promise.all([
          fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}`),
          fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/play`),
        ])
        return { detail: normalizeVideoDetail(detailPayload), play: normalizeVideoPlay(playPayload), adminPreview: false }
      } catch (publicError) {
        if (!session?.user.isAdmin) throw publicError
        const detailResult = await authorizedJson<unknown>(`/api/v1/admin/videos/${encodeURIComponent(bvid)}`, {}, session)
        const playResult = await authorizedJson<unknown>(`/api/v1/admin/videos/${encodeURIComponent(bvid)}/play`, {}, detailResult.session)
        if (playResult.session.accessToken !== session.accessToken) setSession(playResult.session)
        return { detail: normalizeVideoDetail(detailResult.data), play: normalizeVideoPlay(playResult.data), adminPreview: true }
      }
    }
    void loadVideo().then((result) => {
      if (!active) return
      setState({ status: 'ready', ...result })
      setSelectedStreamId((result.play.streams.find((stream) => stream.defaultStream) ?? result.play.streams[0])?.id ?? '')
    }).catch((loadError) => {
      if (active) setState({ status: 'error', message: toErrorMessage(loadError, '视频加载失败') })
    })
    return () => { active = false }
  }, [bvid, restoring, session, setSession])

  useEffect(() => {
    let active = true
    void fetchJson<unknown>('/api/v1/videos?page_size=9')
      .then(normalizeVideoList)
      .then((page) => {
        if (active) setRelatedVideos(page.videos.filter((video) => video.bvid !== bvid).slice(0, 8))
      })
      .catch(() => {
        if (active) setRelatedVideos([])
      })
    return () => { active = false }
  }, [bvid])

  useEffect(() => {
    let active = true
    if (!session || state.status !== 'ready' || adminPreviewMode) {
      const clear = window.setTimeout(() => {
        if (active) setEngagement(null)
      }, 0)
      return () => {
        active = false
        window.clearTimeout(clear)
      }
    }
    void authorizedJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/engagement`, {}, session)
      .then((result) => {
        if (!active) return
        setSession(result.session)
        setEngagement(normalizeVideoEngagement(result.data))
      })
      .catch(() => {
        if (active) setEngagement(null)
      })
    return () => { active = false }
  }, [adminPreviewMode, bvid, session, setSession, state.status])

  const selectedStream = useMemo(() => {
    if (state.status !== 'ready') return undefined
    return state.play.streams.find((stream) => stream.id === selectedStreamId) ?? state.play.streams[0]
  }, [selectedStreamId, state])

  const activeDanmaku = useMemo(() => {
    if (state.status !== 'ready' || !danmakuOn) return []
    return (state.play.danmaku?.items ?? []).filter((item) => {
      const age = currentTime - item.timeSeconds
      return age >= 0 && age <= 5
    })
  }, [currentTime, danmakuOn, state])

  useEffect(() => {
    const video = videoRef.current
    if (!video || !selectedStream) return
    const resumeAt = video.currentTime
    const shouldPlay = !video.paused || resumeAt === 0
    let cancelled = false
    dashPlayerRef.current?.destroy()
    void import('dashjs').then(({ Debug, MediaPlayer }) => {
      if (cancelled) return
      const player = MediaPlayer().create()
      dashPlayerRef.current = player
      player.updateSettings({ debug: { logLevel: Debug.LOG_LEVEL_FATAL }, streaming: { cmcd: { enabled: false, applyParametersFromMpd: false, eventTargets: [] } } })
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

  useEffect(() => {
    viewSessionRef.current = ''
    watchedSecondsRef.current = 0
    previousPlaybackTimeRef.current = null
    startingViewRef.current = false
    completingViewRef.current = false
    completedViewRef.current = false
  }, [bvid, session?.user.id])

  async function startViewSession() {
    if (!session || state.status !== 'ready' || state.adminPreview || completedViewRef.current || viewSessionRef.current || startingViewRef.current) return
    startingViewRef.current = true
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/view-sessions`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }, session,
      )
      setSession(result.session)
      viewSessionRef.current = normalizeVideoViewSession(result.data).sessionId
    } catch {
      viewSessionRef.current = ''
    } finally {
      startingViewRef.current = false
    }
  }

  async function completeViewSession() {
    if (!session || state.status !== 'ready' || state.adminPreview || !viewSessionRef.current || completingViewRef.current) return
    completingViewRef.current = true
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/view-sessions/${encodeURIComponent(viewSessionRef.current)}:complete`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }, session,
      )
      setSession(result.session)
      const view = normalizeVideoViewResult(result.data)
      updateDetail((detail) => ({ ...detail, viewCount: view.viewCount }))
      completedViewRef.current = true
      viewSessionRef.current = ''
    } catch {
      completingViewRef.current = false
    }
  }

  function trackPlayback(video: HTMLVideoElement) {
    setCurrentTime(video.currentTime)
    if (!session || state.status !== 'ready' || state.adminPreview || video.paused) {
      previousPlaybackTimeRef.current = video.currentTime
      return
    }
    if (!completedViewRef.current && !viewSessionRef.current) void startViewSession()
    const previous = previousPlaybackTimeRef.current
    const delta = previous === null ? 0 : video.currentTime - previous
    previousPlaybackTimeRef.current = video.currentTime
    if (delta > 0 && delta <= 1.5) watchedSecondsRef.current += delta
    if (!completedViewRef.current && watchedSecondsRef.current >= 5.5) void completeViewSession()
  }

  function requireSession() {
    if (session) return true
    setInteractionMessage('请先点击右上角登录')
    return false
  }

  async function toggleLike() {
    if (!requireSession() || !session || !engagement) return
    const previous = engagement
    const desired = !previous.liked
    setInteractionPending('like')
    setInteractionMessage('')
    setEngagement({ ...previous, liked: desired, likeCount: Math.max(0, toNumber(previous.likeCount) + (desired ? 1 : -1)) })
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/like`,
        { method: desired ? 'POST' : 'DELETE' }, session,
      )
      setSession(result.session)
      const like = normalizeVideoLike(result.data)
      setEngagement((current) => current ? { ...current, liked: like.liked, likeCount: like.likeCount } : current)
    } catch (likeError) {
      setEngagement(previous)
      setInteractionMessage(toErrorMessage(likeError, '操作失败，未保存离线更改'))
    } finally {
      setInteractionPending('')
    }
  }

  async function toggleFavorite() {
    if (!requireSession() || !session || !engagement) return
    const previous = engagement
    const desired = !previous.favorited
    setInteractionPending('favorite')
    setInteractionMessage('')
    setEngagement({ ...previous, favorited: desired, favoriteCount: Math.max(0, toNumber(previous.favoriteCount) + (desired ? 1 : -1)) })
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/favorite`,
        { method: desired ? 'POST' : 'DELETE' }, session,
      )
      setSession(result.session)
      setEngagement(normalizeVideoEngagement(result.data))
    } catch (favoriteError) {
      setEngagement(previous)
      setInteractionMessage(toErrorMessage(favoriteError, '操作失败，未保存离线更改'))
    } finally {
      setInteractionPending('')
    }
  }

  async function coinVideo(targetAmount: number) {
    if (!requireSession() || !session) return
    setInteractionPending('coin')
    setInteractionMessage('')
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/coin`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ target_amount: targetAmount }) }, session,
      )
      const nextEngagement = normalizeVideoEngagement(result.data)
      setSession({ ...result.session, user: { ...result.session.user, coinBalance: nextEngagement.coinBalance } })
      setEngagement(nextEngagement)
      setInteractionMessage(`已累计投出 ${nextEngagement.myCoinAmount} 枚硬币`)
    } catch (coinError) {
      setInteractionMessage(toErrorMessage(coinError, '投币失败'))
    } finally {
      setInteractionPending('')
    }
  }

  async function shareVideo() {
    if (!requireSession() || !session || state.status !== 'ready') return
    setInteractionPending('share')
    setInteractionMessage('')
    try {
      const url = window.location.href
      const usedNativeShare = typeof navigator.share === 'function'
      if (usedNativeShare) await navigator.share({ title: state.detail.title, url })
      else await navigator.clipboard.writeText(url)
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/shares`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ request_id: crypto.randomUUID() }) }, session,
      )
      setSession(result.session)
      const record = result.data as Record<string, unknown>
      setEngagement((current) => current ? { ...current, shareCount: toNumber((record.shareCount ?? record.share_count ?? current.shareCount) as number | string) } : current)
      setInteractionMessage(usedNativeShare ? '分享成功' : '链接已复制')
    } catch (shareError) {
      if ((shareError as DOMException).name !== 'AbortError') setInteractionMessage(toErrorMessage(shareError, '分享失败'))
    } finally {
      setInteractionPending('')
    }
  }

  async function createDanmaku(text: string, color: string) {
    if (!session || state.status !== 'ready' || state.adminPreview) return false
    setDanmakuPending(true)
    setInteractionMessage('')
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/danmakus`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ time_seconds: currentTime, text, color }) }, session,
      )
      setSession(result.session)
      const item = normalizeDanmaku(result.data)
      setState((current) => current.status === 'ready' ? { ...current, play: { ...current.play, danmaku: { enabled: true, format: current.play.danmaku?.format || 'inline', items: [...(current.play.danmaku?.items ?? []), item] } }, detail: { ...current.detail, danmakuCount: toNumber(current.detail.danmakuCount) + 1 } } : current)
      return true
    } catch (danmakuError) {
      setInteractionMessage(toErrorMessage(danmakuError, '弹幕发送失败'))
      return false
    } finally {
      setDanmakuPending(false)
    }
  }

  async function deleteDanmaku(item: DanmakuItem) {
    if (!session || state.status !== 'ready' || state.adminPreview) return
    try {
      const result = await authorizedFetch(`/api/v1/videos/${encodeURIComponent(bvid)}/danmakus/${item.id}`, { method: 'DELETE' }, session)
      setSession(result.session)
      setState((current) => current.status === 'ready' ? { ...current, play: { ...current.play, danmaku: { enabled: true, format: current.play.danmaku?.format || 'inline', items: (current.play.danmaku?.items ?? []).filter((entry) => entry.id !== item.id) } }, detail: { ...current.detail, danmakuCount: Math.max(0, toNumber(current.detail.danmakuCount) - 1) } } : current)
    } catch (deleteError) {
      setInteractionMessage(toErrorMessage(deleteError, '弹幕删除失败'))
    }
  }

  async function takeDownVideo() {
    if (!session?.user.isAdmin || !takeDownReason.trim() || moderationPending) return
    setModerationPending('take-down')
    setInteractionMessage('')
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/admin/videos/${encodeURIComponent(bvid)}/take-down`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ reason: takeDownReason.trim() }),
        },
        session,
      )
      setSession(result.session)
      navigate('/admin/reviews?status=rejected', { replace: true })
    } catch (takeDownError) {
      setInteractionMessage(toErrorMessage(takeDownError, '下架失败'))
      setModerationPending('')
    }
  }

  async function deleteVideoMedia() {
    if (!session?.user.isAdmin || !takeDownReason.trim() || moderationPending) return
    if (!window.confirm(`永久删除 ${bvid} 的 DASH 分片和封面？此操作不可恢复。`)) return
    setModerationPending('delete')
    setInteractionMessage('')
    try {
      const query = new URLSearchParams({ reason: takeDownReason.trim() })
      const result = await authorizedFetch(
        `/api/v1/admin/videos/${encodeURIComponent(bvid)}?${query}`,
        { method: 'DELETE' },
        session,
      )
      setSession(result.session)
      navigate('/admin/reviews?status=rejected', { replace: true })
    } catch (deleteError) {
      setInteractionMessage(toErrorMessage(deleteError, '媒体文件删除失败'))
      setModerationPending('')
    }
  }

  function updateDetail(update: (detail: VideoDetail) => VideoDetail) {
    setState((current) => current.status === 'ready' ? { ...current, detail: update(current.detail) } : current)
  }

  if (state.status === 'loading') return <VideoLoading />
  if (state.status === 'error') return <VideoError bvid={bvid} message={state.message} />

  const { detail, play, adminPreview } = state
  const rejectedPreview = adminPreview && (detail.status?.toLowerCase().includes('rejected') ?? false)
  return (
    <main className="video-page">
      <div className="watch-layout">
        <div className="watch-main">
          <section className="video-info" aria-labelledby="video-title">
            <h1 id="video-title">{detail.title}</h1>
            <div className="video-subline">
              <span><BiliViewIcon size={17} />{formatCount(detail.viewCount)}</span>
              <span><BiliDanmakuIcon size={17} />{formatCount(detail.danmakuCount)}</span>
              <time>{formatDate(detail.publishTime)}</time>
              <span>{detail.bvid}</span>
            </div>
          </section>
          <BiliVideoPlayer
            ref={videoRef}
            poster={detail.coverUrl}
            streams={play.streams}
            selectedStreamId={selectedStream?.id ?? ''}
            danmaku={activeDanmaku}
            danmakuOn={danmakuOn}
            onStreamChange={setSelectedStreamId}
            onDanmakuToggle={() => setDanmakuOn((value) => !value)}
            onPlaying={() => { if (!adminPreview) void startViewSession() }}
            onTimeUpdate={trackPlayback}
            onPause={(video) => { previousPlaybackTimeRef.current = video.currentTime }}
          />
          {!adminPreview && <DanmakuComposer currentTime={currentTime} session={session} pending={danmakuPending} onCreate={createDanmaku} onLoginRequired={() => setInteractionMessage('请先点击右上角登录')} />}
          {adminPreview
            ? <div className="admin-preview-notice"><ShieldAlert size={18} /><span><strong>管理员预览</strong><small>该视频当前不对公众开放，预览不会计入播放历史和互动数据。</small></span></div>
            : <InteractionBar video={detail} engagement={engagement} pending={interactionPending} message={interactionMessage} onLike={() => void toggleLike()} onFavorite={() => void toggleFavorite()} onCoin={(amount) => void coinVideo(amount)} onShare={() => void shareVideo()} />}
          <section className="video-description"><p>{detail.description || '作者没有填写视频简介。'}</p>{detail.tags.length > 0 && <div className="tag-list">{detail.tags.map((tag) => <span key={tag}>{tag}</span>)}</div>}</section>
          {session?.user.isAdmin && !adminPreview && (
            <section className="video-admin-panel" aria-label="管理员操作">
              <div className="video-admin-heading"><ShieldAlert size={18} /><span><strong>内容管理</strong><small>仅下架会保留媒体供复核；永久删除文件后不可恢复播放。</small></span></div>
              <input value={takeDownReason} maxLength={1000} onChange={(event) => setTakeDownReason(event.target.value)} placeholder="填写下架或删除原因" />
              <div className="video-admin-actions">
                <button type="button" className="take-down-only" disabled={!takeDownReason.trim() || Boolean(moderationPending)} onClick={() => void takeDownVideo()}>{moderationPending === 'take-down' ? '处理中' : '仅下架'}</button>
                <button type="button" className="delete-media" disabled={!takeDownReason.trim() || Boolean(moderationPending)} onClick={() => void deleteVideoMedia()}><Trash2 size={16} />{moderationPending === 'delete' ? '正在删除' : '下架并删除文件'}</button>
              </div>
            </section>
          )}
          {session?.user.isAdmin && rejectedPreview && (
            <section className="video-admin-panel admin-preview-cleanup" aria-label="已下架视频清理">
              <div className="video-admin-heading"><ShieldAlert size={18} /><span><strong>视频已下架</strong><small>{detail.reviewReason || 'DASH 分片仍保留，仅管理员可预览。'}</small></span></div>
              <input value={takeDownReason} maxLength={1000} onChange={(event) => setTakeDownReason(event.target.value)} placeholder="填写永久删除原因" />
              <div className="video-admin-actions">
                <button type="button" className="delete-media" disabled={!takeDownReason.trim() || Boolean(moderationPending)} onClick={() => void deleteVideoMedia()}><Trash2 size={16} />{moderationPending === 'delete' ? '正在删除' : '永久删除文件'}</button>
              </div>
            </section>
          )}
          {adminPreview && !rejectedPreview && (
            <div className="admin-review-link"><span>审核操作统一在内容管理页完成。</span><Link to="/admin/reviews">返回待审核队列</Link></div>
          )}
          {!adminPreview && <CommentSection key={bvid} bvid={bvid} ownerId={detail.ownerId} onCountChange={(delta) => updateDetail((video) => ({ ...video, commentCount: Math.max(0, toNumber(video.commentCount) + delta) }))} />}
        </div>

        <aside className="watch-side">
          <Link className="owner-panel" to={`/space/${detail.ownerId}`}><span className="owner-avatar">{detail.ownerAvatarUrl ? <img src={detail.ownerAvatarUrl} alt="" /> : detail.ownerName.slice(0, 1)}</span><div><strong>{detail.ownerName}</strong><p>查看作者主页</p></div></Link>
          {!adminPreview && <DanmakuPanel items={play.danmaku?.items ?? []} session={session} videoOwnerId={detail.ownerId} onDelete={(item) => void deleteDanmaku(item)} />}
          {relatedVideos.length > 0 && <RelatedVideos videos={relatedVideos} />}
        </aside>
      </div>
    </main>
  )
}

function VideoLoading() {
  return <main className="video-page"><div className="watch-layout loading"><div className="player-skeleton" /><div className="side-skeleton" /></div></main>
}

function VideoError({ bvid, message }: { bvid: string; message: string }) {
  return <main className="error-page"><AlertCircle size={34} /><span>{bvid}</span><h1>视频加载失败</h1><p>{message}</p><Link to="/"><ChevronLeft size={17} />返回首页</Link></main>
}
