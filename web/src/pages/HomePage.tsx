import { Clapperboard, Gamepad2, Music2, Radio, RefreshCw, Sparkles, Upload } from 'lucide-react'
import { useEffect, useState } from 'react'
import { fetchJson, normalizeVideoList, toErrorMessage } from '../api'
import { VideoCard } from '../components/VideoCard'
import { useUploadPanel } from '../context/UploadContext'
import type { VideoDetail } from '../types'

const channels = [
  { label: '动画', icon: Clapperboard },
  { label: '音乐', icon: Music2 },
  { label: '游戏', icon: Gamepad2 },
  { label: '知识', icon: Sparkles },
  { label: '直播', icon: Radio },
]

export function HomePage() {
  const openUpload = useUploadPanel()
  const [videos, setVideos] = useState<VideoDetail[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    void loadPage('').then((page) => {
      if (!active) return
      setVideos(page.videos)
      setNextPageToken(page.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '视频列表加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [])

  async function loadMore() {
    if (!nextPageToken || loadingMore) return
    setLoadingMore(true)
    try {
      const page = await loadPage(nextPageToken)
      setVideos((current) => [...current, ...page.videos])
      setNextPageToken(page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页加载失败'))
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <main className="home-page">
      <section className="channel-strip" aria-label="内容分区">
        <div className="channel-inner">
          {channels.map(({ label, icon: Icon }) => <span className="channel-chip" key={label}><Icon size={18} /><span>{label}</span></span>)}
          <span className="channel-divider" />
          <button type="button" onClick={openUpload}><Upload size={18} /><span>投稿</span></button>
        </div>
      </section>

      <section className="feed-section" aria-labelledby="feed-title">
        <header className="feed-heading"><div><h1 id="feed-title">推荐</h1><p>最近发布的视频</p></div><span>{videos.length} 个视频</span></header>
        {loading ? (
          <div className="video-grid" aria-label="正在加载视频">
            {Array.from({ length: 10 }, (_, index) => <div className="video-skeleton" key={index} />)}
          </div>
        ) : videos.length > 0 ? (
          <>
            <div className="video-grid">{videos.map((video) => <VideoCard key={video.bvid} video={video} />)}</div>
            {nextPageToken && <button className="load-more-button" type="button" disabled={loadingMore} onClick={() => void loadMore()}><RefreshCw size={17} />{loadingMore ? '加载中' : '加载更多'}</button>}
          </>
        ) : (
          <div className="home-empty">
            <span className="empty-mark"><Clapperboard size={30} /></span>
            <h2>{error ? '暂时无法读取视频' : '还没有视频'}</h2>
            <p>{error || '上传第一支视频后，它会出现在这里。'}</p>
            <button className="primary-button" type="button" onClick={openUpload}><Upload size={18} />投稿视频</button>
          </div>
        )}
        {error && videos.length > 0 && <p className="inline-error" role="status">{error}</p>}
      </section>
    </main>
  )
}

async function loadPage(pageToken: string) {
  const query = new URLSearchParams({ page_size: '12' })
  if (pageToken) query.set('page_token', pageToken)
  return normalizeVideoList(await fetchJson<unknown>(`/api/v1/videos?${query}`))
}
