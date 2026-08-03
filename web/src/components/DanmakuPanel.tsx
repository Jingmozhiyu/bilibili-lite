import { ChevronDown, Trash2 } from 'lucide-react'
import { useState } from 'react'
import type { AuthSession, DanmakuItem } from '../types'
import { formatPlaybackTime } from '../utils/format'

type DanmakuPanelProps = {
  items: DanmakuItem[]
  session: AuthSession | null
  videoOwnerId?: number
  onDelete: (item: DanmakuItem) => void
}

export function DanmakuPanel({ items, session, videoOwnerId, onDelete }: DanmakuPanelProps) {
  const [expanded, setExpanded] = useState(false)

  return (
    <section className={`danmaku-panel ${expanded ? 'expanded' : ''}`} aria-labelledby="danmaku-panel-title">
      <button type="button" className="danmaku-panel-toggle" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>
        <span><strong id="danmaku-panel-title">弹幕列表</strong><small>{items.length} 条</small></span>
        <ChevronDown size={17} />
      </button>
      {expanded && <div className="danmaku-table">
        <div className="danmaku-table-head"><span>时间</span><span>弹幕内容</span><span>发送时间</span></div>
        <div className="danmaku-list">
          {items.length === 0 ? <p className="compact-empty">还没有弹幕</p> : items.map((item) => {
            const canDelete = session && (session.user.id === item.userId || session.user.id === videoOwnerId)
            return <div className="danmaku-row" key={item.id || `${item.timeSeconds}-${item.text}`}>
              <time>{formatPlaybackTime(item.timeSeconds)}</time>
              <span title={item.text} style={{ color: item.color === '#ffffff' ? undefined : item.color }}>{item.text}</span>
              <small>{formatSendTime(item.createdAt)}</small>
              {canDelete && <button type="button" className="danmaku-delete" title="删除弹幕" aria-label="删除弹幕" onClick={() => onDelete(item)}><Trash2 size={14} /></button>}
            </div>
          })}
        </div>
      </div>}
    </section>
  )
}

function formatSendTime(value?: string) {
  if (!value) return '--:--'
  return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value))
}
