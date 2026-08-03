import { Send } from 'lucide-react'
import { useState } from 'react'
import type { AuthSession } from '../types'
import { formatPlaybackTime } from '../utils/format'

const danmakuColors = ['#ffffff', '#00aeec', '#fb7299', '#ffd166']

type DanmakuComposerProps = {
  currentTime: number
  session: AuthSession | null
  pending: boolean
  onCreate: (text: string, color: string) => Promise<boolean>
  onLoginRequired: () => void
}

export function DanmakuComposer({ currentTime, session, pending, onCreate, onLoginRequired }: DanmakuComposerProps) {
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
    <form className="danmaku-composer" onSubmit={submit}>
      <span className="danmaku-current-time">{formatPlaybackTime(currentTime)}</span>
      <div className="color-swatches" aria-label="弹幕颜色">
        {danmakuColors.map((value) => <button type="button" key={value} className={color === value ? 'selected' : ''} aria-label={`颜色 ${value}`} style={{ backgroundColor: value }} onClick={() => setColor(value)} />)}
      </div>
      <input maxLength={100} value={text} onChange={(event) => setText(event.target.value)} placeholder={session ? '发个友善的弹幕见证当下' : '登录后发送弹幕'} />
      <button type="submit" className="send-button" disabled={pending || !text.trim()} title="发送弹幕"><Send size={17} />发送</button>
    </form>
  )
}
