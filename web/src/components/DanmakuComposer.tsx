import { Send } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { AuthSession } from '../types'
import { BiliDanmakuSettingsIcon } from './BiliIcons'

const danmakuColors = ['#ffffff', '#00aeec', '#fb7299', '#ffd166']

type DanmakuComposerProps = {
  session: AuthSession | null
  pending: boolean
  speed: number
  onCreate: (text: string, color: string) => Promise<boolean>
  onLoginRequired: () => void
  onSpeedChange: (speed: number) => void
}

export function DanmakuComposer({ session, pending, speed, onCreate, onLoginRequired, onSpeedChange }: DanmakuComposerProps) {
  const [text, setText] = useState('')
  const [color, setColor] = useState('#ffffff')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const settingsRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!settingsOpen) return
    function closeSettings(event: MouseEvent) {
      if (!settingsRef.current?.contains(event.target as Node)) setSettingsOpen(false)
    }
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') setSettingsOpen(false)
    }
    document.addEventListener('pointerdown', closeSettings)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', closeSettings)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [settingsOpen])

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
      <div className="danmaku-settings" ref={settingsRef}>
        <button type="button" className={settingsOpen ? 'danmaku-settings-trigger active' : 'danmaku-settings-trigger'} aria-label="弹幕设置" title="弹幕设置" aria-expanded={settingsOpen} aria-controls="danmaku-settings-panel" onClick={() => setSettingsOpen((open) => !open)}>
          <BiliDanmakuSettingsIcon />
        </button>
        {settingsOpen && <section id="danmaku-settings-panel" className="danmaku-settings-panel" role="dialog" aria-label="弹幕设置">
          <header><strong>弹幕设置</strong><output>{speed.toFixed(2)}×</output></header>
          <label className="danmaku-speed-setting">
            <span>流速</span>
            <input type="range" min="0.75" max="2" step="0.25" value={speed} onChange={(event) => onSpeedChange(Number(event.target.value))} />
          </label>
          <div className="danmaku-speed-scale" aria-hidden="true"><span>慢</span><span>标准</span><span>快</span></div>
          <div className="danmaku-color-setting">
            <span>颜色</span>
            <div className="color-swatches" aria-label="弹幕颜色">
              {danmakuColors.map((value) => <button type="button" key={value} className={color === value ? 'selected' : ''} aria-label={`颜色 ${value}`} aria-pressed={color === value} style={{ backgroundColor: value }} onClick={() => setColor(value)} />)}
            </div>
          </div>
        </section>}
      </div>
      <input maxLength={100} value={text} onChange={(event) => setText(event.target.value)} placeholder={session ? '发个友善的弹幕见证当下' : '登录后发送弹幕'} />
      <button type="submit" className="send-button" disabled={pending || !text.trim()} title="发送弹幕"><Send size={17} />发送</button>
    </form>
  )
}
