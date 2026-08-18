import { Maximize, Pause, Play, Volume2, VolumeX } from 'lucide-react'
import { forwardRef, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, MutableRefObject } from 'react'
import type { DanmakuItem, VideoStream } from '../types'
import { formatDuration } from '../utils/format'
import { BiliDanmakuIcon } from './BiliIcons'

type BiliVideoPlayerProps = {
  poster?: string
  streams: VideoStream[]
  qualityOptions: Array<{ id: string; label: string }>
  selectedQualityId: string
  danmaku: DanmakuItem[]
  danmakuOn: boolean
  danmakuSpeed: number
  onQualityChange: (qualityID: string) => void
  onDanmakuToggle: () => void
  onPlaying: (video: HTMLVideoElement) => void
  onTimeUpdate: (video: HTMLVideoElement) => void
  onPause: (video: HTMLVideoElement) => void
}

export const BiliVideoPlayer = forwardRef<HTMLVideoElement, BiliVideoPlayerProps>(function BiliVideoPlayer({
  poster,
  streams,
  qualityOptions,
  selectedQualityId,
  danmaku,
  danmakuOn,
  danmakuSpeed,
  onQualityChange,
  onDanmakuToggle,
  onPlaying,
  onTimeUpdate,
  onPause,
}, forwardedRef) {
  const shellRef = useRef<HTMLElement | null>(null)
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const [playing, setPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(1)
  const [muted, setMuted] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const [playerWidth, setPlayerWidth] = useState(0)
  const [danmakuTime, setDanmakuTime] = useState(0)

  const assignVideoRef = useCallback((node: HTMLVideoElement | null) => {
    videoRef.current = node
    if (typeof forwardedRef === 'function') forwardedRef(node)
    else if (forwardedRef) (forwardedRef as MutableRefObject<HTMLVideoElement | null>).current = node
  }, [forwardedRef])

  useEffect(() => {
    function syncFullscreen() {
      setFullscreen(document.fullscreenElement === shellRef.current)
    }
    document.addEventListener('fullscreenchange', syncFullscreen)
    return () => document.removeEventListener('fullscreenchange', syncFullscreen)
  }, [])

  useEffect(() => {
    if (!playing || !danmakuOn || danmaku.length === 0) {
      setDanmakuTime(videoRef.current?.currentTime ?? 0)
      return
    }
    let frame = 0
    const updateDanmakuClock = () => {
      setDanmakuTime(videoRef.current?.currentTime ?? 0)
      frame = window.requestAnimationFrame(updateDanmakuClock)
    }
    frame = window.requestAnimationFrame(updateDanmakuClock)
    return () => window.cancelAnimationFrame(frame)
  }, [danmaku.length, danmakuOn, playing])

  useEffect(() => {
    const shell = shellRef.current
    if (!shell) return
    const updateWidth = () => setPlayerWidth(shell.clientWidth)
    updateWidth()
    const observer = new ResizeObserver(updateWidth)
    observer.observe(shell)
    return () => observer.disconnect()
  }, [])

  async function togglePlayback() {
    const video = videoRef.current
    if (!video || streams.length === 0) return
    if (video.paused) await video.play()
    else video.pause()
  }

  function seek(value: number) {
    const video = videoRef.current
    if (!video || !Number.isFinite(value)) return
    video.currentTime = value
    setCurrentTime(value)
    setDanmakuTime(value)
  }

  function changeVolume(value: number) {
    const video = videoRef.current
    if (!video) return
    video.volume = value
    video.muted = value === 0
    setVolume(value)
    setMuted(value === 0)
  }

  function toggleMute() {
    const video = videoRef.current
    if (!video) return
    video.muted = !video.muted
    setMuted(video.muted)
  }

  async function toggleFullscreen() {
    if (!shellRef.current) return
    if (document.fullscreenElement) await document.exitFullscreen()
    else await shellRef.current.requestFullscreen()
  }

  useEffect(() => {
    function handleKeyboard(event: globalThis.KeyboardEvent) {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || isEditableTarget(event.target)) return
      const key = event.key.toLowerCase()
      if (event.code === 'Space' || key === 'k') {
        if (event.repeat) return
        event.preventDefault()
        void togglePlayback()
      } else if (key === 'm') {
        if (event.repeat) return
        event.preventDefault()
        toggleMute()
      } else if (key === 'f') {
        if (event.repeat) return
        event.preventDefault()
        void toggleFullscreen()
      } else if (event.key === 'ArrowLeft') {
        event.preventDefault()
        seek(Math.max(0, currentTime - 5))
      } else if (event.key === 'ArrowRight') {
        event.preventDefault()
        seek(Math.min(duration, currentTime + 5))
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        changeVolume(Math.min(1, (muted ? 0 : volume) + 0.05))
      } else if (event.key === 'ArrowDown') {
        event.preventDefault()
        changeVolume(Math.max(0, (muted ? 0 : volume) - 0.05))
      }
    }
    document.addEventListener('keydown', handleKeyboard)
    return () => document.removeEventListener('keydown', handleKeyboard)
  })

  const progress = duration > 0 ? Math.min(100, (currentTime / duration) * 100) : 0
  const progressStyle = { '--player-progress': `${progress}%` } as CSSProperties
  const visibleDanmaku = useMemo(() => danmaku.flatMap((item, index) => {
    const lifetime = danmakuDuration(playerWidth, item.text, danmakuSpeed)
    const age = danmakuTime - item.timeSeconds
    if (!danmakuOn || age < 0 || age >= lifetime) return []
    const textWidth = estimatedDanmakuWidth(playerWidth, item.text)
    const x = playerWidth - (age / lifetime) * (playerWidth + textWidth)
    return [{ item, index, x }]
  }), [danmaku, danmakuOn, danmakuSpeed, danmakuTime, playerWidth])

  return (
    <section ref={shellRef} className={`player-shell ${playing ? 'is-playing' : 'is-paused'} ${fullscreen ? 'is-fullscreen' : ''}`} aria-label="视频播放器" tabIndex={0}>
      {streams.length === 0 && <div className="player-empty"><strong>暂无可播放的视频流</strong><span>视频资源可能仍在处理中</span></div>}
      <video
        ref={assignVideoRef}
        playsInline
        poster={poster}
        onClick={() => void togglePlayback()}
        onLoadedMetadata={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
        onDurationChange={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
        onPlaying={(event) => { setPlaying(true); onPlaying(event.currentTarget) }}
        onPause={(event) => { setPlaying(false); setDanmakuTime(event.currentTarget.currentTime); onPause(event.currentTarget) }}
        onEnded={() => setPlaying(false)}
        onSeeking={(event) => setDanmakuTime(event.currentTarget.currentTime)}
        onSeeked={(event) => setDanmakuTime(event.currentTarget.currentTime)}
        onTimeUpdate={(event) => { setCurrentTime(event.currentTarget.currentTime); setDanmakuTime(event.currentTarget.currentTime); onTimeUpdate(event.currentTarget) }}
        onVolumeChange={(event) => { setVolume(event.currentTarget.volume); setMuted(event.currentTarget.muted) }}
      />

      <div className="danmaku-stage" aria-hidden="true">{visibleDanmaku.map(({ item, index, x }) => <span key={`${item.id}-${index}`} style={{ top: `${10 + (index % 7) * 11}%`, color: item.color, transform: `translate3d(${x}px, 0, 0)` }}>{item.text}</span>)}</div>

      {!playing && streams.length > 0 && <button type="button" className="player-center-play" aria-label="播放" title="播放" onClick={() => void togglePlayback()}><Play size={32} fill="currentColor" /></button>}

      <div className="bili-player-controls">
        <input className="player-progress" aria-label="播放进度" type="range" min="0" max={duration || 0} step="0.01" value={Math.min(currentTime, duration || 0)} style={progressStyle} onChange={(event) => seek(Number(event.target.value))} />
        <div className="player-control-row">
          <button type="button" aria-label={playing ? '暂停' : '播放'} title={playing ? '暂停' : '播放'} onClick={() => void togglePlayback()}>{playing ? <Pause size={20} fill="currentColor" /> : <Play size={20} fill="currentColor" />}</button>
          <time>{formatDuration(currentTime)} / {formatDuration(duration)}</time>
          <div className="player-volume-control">
            <button type="button" aria-label={muted ? '取消静音' : '静音'} title={muted ? '取消静音' : '静音'} onClick={toggleMute}>{muted || volume === 0 ? <VolumeX size={20} /> : <Volume2 size={20} />}</button>
            <input aria-label="音量" type="range" min="0" max="1" step="0.05" value={muted ? 0 : volume} onChange={(event) => changeVolume(Number(event.target.value))} />
          </div>
          <span className="player-controls-spacer" />
          <button type="button" className={danmakuOn ? 'active' : ''} aria-label={danmakuOn ? '关闭弹幕' : '打开弹幕'} title={danmakuOn ? '关闭弹幕' : '打开弹幕'} onClick={onDanmakuToggle}><BiliDanmakuIcon size={21} /></button>
          {streams.length > 0 && <label className="player-quality" title="清晰度"><span className="sr-only">清晰度</span><select aria-label="清晰度" value={selectedQualityId} onChange={(event) => onQualityChange(event.target.value)}>{qualityOptions.map((quality) => <option key={quality.id} value={quality.id}>{quality.label}</option>)}</select></label>}
          <button type="button" aria-label={fullscreen ? '退出全屏' : '全屏'} title={fullscreen ? '退出全屏' : '全屏'} onClick={() => void toggleFullscreen()}><Maximize size={20} /></button>
        </div>
      </div>
    </section>
  )
})

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || !!target.closest('input, textarea, select, button, a, [role="button"], [contenteditable="true"]')
}

function danmakuDuration(playerWidth: number, text: string, speed: number) {
  const width = playerWidth > 0 ? playerWidth : 960
  const textWidth = estimatedDanmakuWidth(width, text)
  return Math.min(14, Math.max(3.5, (width + textWidth) / (120 * speed)))
}

function estimatedDanmakuWidth(playerWidth: number, text: string) {
  const width = playerWidth > 0 ? playerWidth : 960
  return Math.min(width * 0.8, Math.max(36, Array.from(text).length * 18))
}
