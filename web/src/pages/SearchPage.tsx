import { Search, SearchX } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { fetchJson, normalizeVideoSearch, toErrorMessage } from '../api'
import { VideoCard } from '../components/VideoCard'
import type { VideoDetail } from '../types'

const searchOrders = [
  { id: 'relevance', label: '综合排序', value: '1' },
  { id: 'views', label: '最多播放', value: '2' },
  { id: 'latest', label: '最新发布', value: '3' },
  { id: 'danmaku', label: '最多弹幕', value: '4' },
  { id: 'favorites', label: '最多收藏', value: '5' },
] as const

export function SearchPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const keyword = searchParams.get('keyword')?.trim() || ''
  const selectedOrder = searchOrders.some((item) => item.id === searchParams.get('order'))
    ? searchParams.get('order')!
    : 'relevance'

  function changeOrder(order: string) {
    if (keyword) setSearchParams({ keyword, order })
  }

  return (
    <main className="search-page">
      <SearchForm key={keyword} initialValue={keyword} onSearch={(query) => setSearchParams({ keyword: query, order: selectedOrder })} />

      {keyword && (
        <div className="search-result-bar">
          <nav aria-label="搜索排序">
            {searchOrders.map((order) => (
              <button type="button" key={order.id} className={selectedOrder === order.id ? 'active' : ''} onClick={() => changeOrder(order.id)}>
                {order.label}
              </button>
            ))}
          </nav>
        </div>
      )}

      {!keyword ? (
        <SearchEmpty title="输入关键词开始搜索" detail="支持标题、简介、标签、作者和 BV 号" />
      ) : (
        <SearchResults key={`${keyword}-${selectedOrder}`} keyword={keyword} order={selectedOrder} />
      )}
    </main>
  )
}

function SearchForm({ initialValue, onSearch }: { initialValue: string; onSearch: (query: string) => void }) {
  const [draft, setDraft] = useState(initialValue)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const query = draft.trim()
    if (query) onSearch(query)
  }

  return (
    <form className="search-page-form" role="search" onSubmit={submit}>
      <Search size={20} aria-hidden="true" />
      <input aria-label="搜索视频" autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="搜索视频、作者或 BV 号" />
      <button type="submit" disabled={!draft.trim()}>搜索</button>
    </form>
  )
}

function SearchResults({ keyword, order }: { keyword: string; order: string }) {
  const [videos, setVideos] = useState<VideoDetail[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [totalHits, setTotalHits] = useState(0)
  const [processingTime, setProcessingTime] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    void loadSearch(keyword, order, '').then((page) => {
      if (!active) return
      setVideos(page.videos)
      setNextPageToken(page.nextPageToken)
      setTotalHits(page.totalHits)
      setProcessingTime(page.processingTimeMs)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '搜索暂时不可用'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [keyword, order])

  async function loadMore() {
    if (!nextPageToken || loadingMore) return
    setLoadingMore(true)
    try {
      const page = await loadSearch(keyword, order, nextPageToken)
      setVideos((current) => [...current, ...page.videos])
      setNextPageToken(page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页加载失败'))
    } finally {
      setLoadingMore(false)
    }
  }

  if (loading) return <div className="search-grid">{Array.from({ length: 15 }, (_, index) => <div className="video-skeleton" key={index} />)}</div>
  if (error && videos.length === 0) return <SearchEmpty title="搜索暂时不可用" detail={error} />
  if (videos.length === 0) return <SearchEmpty title={`没有找到“${keyword}”`} detail="换个关键词或检查输入内容" />
  return (
    <>
      <p className="search-result-summary">找到 {totalHits} 个结果 · {processingTime} ms</p>
      <div className="search-grid">{videos.map((video) => <VideoCard key={video.bvid} video={video} />)}</div>
      {nextPageToken && <button type="button" className="load-more-button" disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? '加载中' : '加载更多'}</button>}
      {error && <p className="inline-error" role="status">{error}</p>}
    </>
  )
}

function SearchEmpty({ title, detail }: { title: string; detail: string }) {
  return <div className="search-empty"><SearchX size={34} /><h1>{title}</h1><p>{detail}</p></div>
}

async function loadSearch(keyword: string, orderID: string, pageToken: string) {
  const order = searchOrders.find((item) => item.id === orderID) || searchOrders[0]
  const query = new URLSearchParams({ query: keyword, order: order.value, page_size: '20' })
  if (pageToken) query.set('page_token', pageToken)
  return normalizeVideoSearch(await fetchJson<unknown>(`/api/v1/search/videos?${query}`))
}
