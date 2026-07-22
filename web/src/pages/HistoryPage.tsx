import { Coins, Heart, Library, LogIn } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { authorizedJson, normalizeVideoHistory, toErrorMessage } from '../api'
import { useAuth } from '../auth/useAuth'
import { VideoCard } from '../components/VideoCard'
import type { VideoHistoryItem } from '../types'
import { formatDate } from '../utils/format'

const historyConfig = {
  likes: { endpoint: 'video-likes', title: '点赞历史', description: '当前仍保持点赞的视频', icon: Heart },
  favorites: { endpoint: 'video-favorites', title: '收藏历史', description: '当前仍在收藏中的视频', icon: Library },
  coins: { endpoint: 'video-coins', title: '投币历史', description: '投币不可撤销', icon: Coins },
} as const

export function HistoryPage() {
  const { kind = 'likes' } = useParams()
  const { session } = useAuth()
  return <HistoryContent key={`${kind}:${session?.user.id ?? 0}`} kind={kind} />
}

function HistoryContent({ kind }: { kind: string }) {
  const { session, restoring, setSession } = useAuth()
  const config = historyConfig[kind as keyof typeof historyConfig] || historyConfig.likes
  const [items, setItems] = useState<VideoHistoryItem[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const Icon = useMemo(() => config.icon, [config])

  const loadHistory = useCallback(async (pageToken: string) => {
    if (!session) throw new Error('请先登录')
    const query = new URLSearchParams({ page_size: '20' })
    if (pageToken) query.set('page_token', pageToken)
    const result = await authorizedJson<unknown>(`/api/v1/users/me/${config.endpoint}?${query}`, {}, session)
    setSession(result.session)
    return normalizeVideoHistory(result.data)
  }, [config.endpoint, session, setSession])

  useEffect(() => {
    if (restoring) return
    if (!session) {
      const finish = window.setTimeout(() => setLoading(false), 0)
      return () => window.clearTimeout(finish)
    }
    let active = true
    void loadHistory('').then((page) => {
      if (!active) return
      setItems(page.items)
      setNextPageToken(page.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '互动历史加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [loadHistory, restoring, session])

  async function loadMore() {
    try {
      const page = await loadHistory(nextPageToken)
      setItems((current) => [...current, ...page.items])
      setNextPageToken(page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页加载失败'))
    }
  }

  if (!restoring && !session) {
    return <main className="history-page"><div className="history-empty"><LogIn size={30} /><h1>登录后查看互动历史</h1><p>点击右上角登录，再回到这里。</p></div></main>
  }

  return (
    <main className="history-page">
      <header className="history-heading"><span><Icon size={24} /></span><div><h1>{config.title}</h1><p>{config.description}</p></div></header>
      {loading ? <div className="video-grid">{Array.from({ length: 8 }, (_, index) => <div className="video-skeleton" key={index} />)}</div>
        : items.length > 0 ? <><div className="video-grid">{items.map((item) => <VideoCard key={`${item.video.bvid}-${item.interactedAt}`} video={item.video} historyLabel={`${kind === 'coins' ? `${item.coinAmount} 币 · ` : ''}${formatDate(item.interactedAt)}`} />)}</div>{nextPageToken && <button className="load-more-button" type="button" onClick={() => void loadMore()}>加载更多</button>}</>
          : <div className="history-empty"><Icon size={30} /><h2>这里还是空的</h2><p>{error || '在视频详情页完成互动后会显示在这里。'}</p></div>}
      {error && items.length > 0 && <p className="inline-error">{error}</p>}
    </main>
  )
}
