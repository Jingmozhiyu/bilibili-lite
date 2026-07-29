import { Clock3, Star } from 'lucide-react'
import { useRef, useState } from 'react'
import type { FocusEvent } from 'react'
import { Link } from 'react-router-dom'
import { authorizedJson, normalizeVideoHistory, toErrorMessage } from '../api'
import { useAuth } from '../auth/useAuth'
import type { VideoHistoryItem } from '../types'
import { formatDuration, formatShortDate } from '../utils/format'

type HeaderHistoryPreviewProps = {
  kind: 'favorites' | 'views'
  open: boolean
  onOpenChange: (open: boolean) => void
  onLoginRequired: () => void
}

const hoverOpenDelay = 240

const previewConfig = {
  favorites: {
    label: '收藏',
    title: '我的收藏',
    endpoint: 'video-favorites',
    target: '/space/me?tab=favorites',
    icon: Star,
  },
  views: {
    label: '历史',
    title: '观看历史',
    endpoint: 'watch-history',
    target: '/history/views',
    icon: Clock3,
  },
} as const

export function HeaderHistoryPreview({ kind, open, onOpenChange, onLoginRequired }: HeaderHistoryPreviewProps) {
  const { session, setSession } = useAuth()
  const [items, setItems] = useState<VideoHistoryItem[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const openTimer = useRef<number | null>(null)
  const closeTimer = useRef<number | null>(null)
  const requestID = useRef(0)
  const config = previewConfig[kind]
  const Icon = config.icon

  function cancelOpen() {
    if (openTimer.current !== null) {
      window.clearTimeout(openTimer.current)
      openTimer.current = null
    }
  }

  function cancelClose() {
    if (closeTimer.current !== null) {
      window.clearTimeout(closeTimer.current)
      closeTimer.current = null
    }
  }

  function scheduleClose() {
    cancelOpen()
    cancelClose()
    closeTimer.current = window.setTimeout(() => onOpenChange(false), 140)
  }

  function openPreview() {
    cancelOpen()
    cancelClose()
    if (!open) {
      onOpenChange(true)
      if (session) void loadPreview()
    }
  }

  function scheduleOpen() {
    if (open) return
    cancelOpen()
    cancelClose()
    openTimer.current = window.setTimeout(openPreview, hoverOpenDelay)
  }

  async function loadPreview() {
    if (!session) return
    const activeRequest = ++requestID.current
    setLoading(true)
    setError('')
    try {
      const query = new URLSearchParams({ page_size: '6' })
      const result = await authorizedJson<unknown>(`/api/v1/users/me/${config.endpoint}?${query}`, {}, session)
      if (activeRequest !== requestID.current) return
      if (result.session.accessToken !== session.accessToken) setSession(result.session)
      setItems(normalizeVideoHistory(result.data).items)
    } catch (loadError) {
      if (activeRequest === requestID.current) setError(toErrorMessage(loadError, `${config.title}加载失败`))
    } finally {
      if (activeRequest === requestID.current) setLoading(false)
    }
  }

  function handleBlur(event: FocusEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) scheduleClose()
  }

  return (
    <div className="header-preview" onMouseEnter={scheduleOpen} onMouseLeave={scheduleClose} onFocus={openPreview} onBlur={handleBlur}>
      <Link
        className="header-action-link"
        to={session ? config.target : '/'}
        aria-label={kind === 'favorites' ? '收藏' : '观看历史'}
        aria-expanded={open}
        onClick={(event) => {
          onOpenChange(false)
          if (!session) {
            event.preventDefault()
            onLoginRequired()
          }
        }}
      >
        <Icon size={19} /><span>{config.label}</span>
      </Link>

      {open && (
        <section className="header-preview-popover" aria-label={config.title}>
          <header><strong>{config.title}</strong></header>
          {!session ? (
            <div className="header-preview-empty"><p>登录后查看{config.label}</p><button type="button" onClick={onLoginRequired}>立即登录</button></div>
          ) : loading ? (
            <div className="header-preview-list" aria-label={`正在加载${config.title}`}>
              {Array.from({ length: 4 }, (_, index) => <span className="header-preview-skeleton" key={index} />)}
            </div>
          ) : items.length > 0 ? (
            <div className="header-preview-list">
              {items.map((item) => (
                <Link className="header-preview-item" to={`/video/${item.video.bvid}`} key={item.video.bvid} onClick={() => onOpenChange(false)}>
                  <span className="header-preview-cover">
                    {item.video.coverUrl ? <img src={item.video.coverUrl} alt="" /> : <b>b</b>}
                    <time>{formatDuration(item.video.durationSeconds)}</time>
                  </span>
                  <span className="header-preview-copy">
                    <strong>{item.video.title}</strong>
                    <small>{kind === 'views' ? `${formatShortDate(item.interactedAt)} · ` : ''}{item.video.ownerName}</small>
                  </span>
                </Link>
              ))}
            </div>
          ) : (
            <div className="header-preview-empty"><p>{error || `暂无${config.label}记录`}</p></div>
          )}
          {session && <Link className="header-preview-footer" to={config.target} onClick={() => onOpenChange(false)}>查看全部</Link>}
        </section>
      )}
    </div>
  )
}
