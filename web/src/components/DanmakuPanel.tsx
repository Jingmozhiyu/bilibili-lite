import { Send, Trash2 } from 'lucide-react'
import { useState } from 'react'
import type { AuthSession, DanmakuItem } from '../types'
import { formatDate } from '../utils/format'

const danmakuColors = ['#ffffff', '#00aeec', '#fb7299', '#ffd166']

type DanmakuPanelProps = {
  items: DanmakuItem[]
  currentTime: number
  session: AuthSession | null
  videoOwnerId?: number
  pending: boolean
  onCreate: (text: string, color: string) => Promise<boolean>
  onDelete: (item: DanmakuItem) => void
  onLoginRequired: () => void
}

export function DanmakuPanel({ items, currentTime, session, videoOwnerId, pending, onCreate, onDelete, onLoginRequired }: DanmakuPanelProps) {
  const [text, setText] = useState('')
  const [color, setColor] = useState('#ffffff')

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!session) {
      onLoginRequired()
      return
    }
    if (!text.trim()) return
    if (await onCreate(text, color)) setText('')
  }

  return (
    <section className="danmaku-panel" aria-labelledby="danmaku-panel-title">
      <header><div><h2 id="danmaku-panel-title">弹幕列表</h2><span>{items.length} 条</span></div><small>当前 {formatPlaybackTime(currentTime)}</small></header>
      <form className="danmaku-composer" onSubmit={submit}>
        <div className="color-swatches" aria-label="弹幕颜色">
          {danmakuColors.map((value) => <button type="button" key={value} className={color === value ? 'selected' : ''} aria-label={`颜色 ${value}`} style={{ backgroundColor: value }} onClick={() => setColor(value)} />)}
        </div>
        <input maxLength={100} value={text} onChange={(event) => setText(event.target.value)} placeholder={session ? '发送一条友善的弹幕' : '登录后发送弹幕'} />
        <button type="submit" className="send-button" disabled={pending || !text.trim()} title="发送弹幕"><Send size={18} /></button>
      </form>
      <div className="danmaku-list">
        {items.length === 0 ? <p className="compact-empty">还没有弹幕，来发第一条吧。</p> : items.map((item) => {
          const canDelete = session && (session.user.id === item.userId || session.user.id === videoOwnerId)
          return <div className="danmaku-row" key={item.id || `${item.timeSeconds}-${item.text}`}>
            <time>{formatPlaybackTime(item.timeSeconds)}</time>
            <span style={{ color: item.color === '#ffffff' ? undefined : item.color }}>{item.text}</span>
            <small>{item.userName || '匿名用户'}{item.createdAt ? ` · ${formatDate(item.createdAt)}` : ''}</small>
            {canDelete && <button type="button" className="icon-button small" title="删除弹幕" aria-label="删除弹幕" onClick={() => onDelete(item)}><Trash2 size={15} /></button>}
          </div>
        })}
      </div>
    </section>
  )
}

function formatPlaybackTime(value: number) {
  const seconds = Math.max(0, Math.floor(value))
  return `${Math.floor(seconds / 60).toString().padStart(2, '0')}:${(seconds % 60).toString().padStart(2, '0')}`
}
