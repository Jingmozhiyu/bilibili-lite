import { Clapperboard, RefreshCw, Upload } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { fetchJson, normalizeVideoList, toErrorMessage } from '../api'
import { HomeCarousel } from '../components/HomeCarousel'
import { HomeChannelNav } from '../components/HomeChannelNav'
import { VideoCard } from '../components/VideoCard'
import { useUploadPanel } from '../context/UploadContext'
import type { VideoDetail } from '../types'

export function HomePage() {
  const openUpload = useUploadPanel()
  const [videos, setVideos] = useState<VideoDetail[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const recommendations = useMemo(() => fillRecommendationSlots(videos, 6), [videos])

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
      <h1 className="sr-only">bilibili-lite 首页</h1>
      <HomeChannelNav />
      <section className="home-showcase" aria-label="推荐内容">
        <HomeCarousel />
        {loading ? (
          <div className="recommend-grid" aria-label="正在加载推荐视频">
            {Array.from({ length: 6 }, (_, index) => <div className="video-skeleton" key={index} />)}
          </div>
        ) : recommendations.length > 0 ? (
          <div className="recommend-grid">
            {recommendations.map((video, index) => <VideoCard video={video} key={`${video.bvid}-${index}`} />)}
          </div>
        ) : (
          <div className="recommend-empty">
            <Clapperboard size={30} />
            <strong>{error ? '推荐暂时不可用' : '还没有推荐视频'}</strong>
            <p>{error || '上传第一支视频后，它会出现在这里。'}</p>
            <button className="primary-button" type="button" onClick={openUpload}><Upload size={18} />投稿视频</button>
          </div>
        )}
      </section>

      {videos.length > 0 && (
        <section className="feed-section" aria-label="视频列表">
          <div className="home-feed-grid">{videos.map((video) => <VideoCard video={video} key={video.bvid} />)}</div>
          {nextPageToken && <button className="load-more-button" type="button" disabled={loadingMore} onClick={() => void loadMore()}><RefreshCw size={17} />{loadingMore ? '加载中' : '加载更多'}</button>}
          {error && <p className="inline-error" role="status">{error}</p>}
        </section>
      )}
    </main>
  )
}

function fillRecommendationSlots(videos: VideoDetail[], size: number) {
  if (videos.length === 0) return []
  return Array.from({ length: size }, (_, index) => videos[index % videos.length])
}

async function loadPage(pageToken: string) {
  const query = new URLSearchParams({ page_size: '12' })
  if (pageToken) query.set('page_token', pageToken)
  return normalizeVideoList(await fetchJson<unknown>(`/api/v1/videos?${query}`))
}
