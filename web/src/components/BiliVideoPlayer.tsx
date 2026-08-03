import { Maximize, Pause, Play, Volume2, VolumeX } from 'lucide-react'
import { forwardRef, useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent, MutableRefObject } from 'react'
import type { DanmakuItem, VideoStream } from '../types'
import { formatDuration } from '../utils/format'
import { BiliDanmakuIcon } from './BiliIcons'

type BiliVideoPlayerProps = {
  poster?: string
  streams: VideoStream[]
  selectedStreamId: string
  danmaku: DanmakuItem[]
  danmakuOn: boolean
  onStreamChange: (streamID: string) => void
  onDanmakuToggle: () => void
  onPlaying: (video: HTMLVideoElement) => void
  onTimeUpdate: (video: HTMLVideoElement) => void
  onPause: (video: HTMLVideoElement) => void
}

export const BiliVideoPlayer = forwardRef<HTMLVideoElement, BiliVideoPlayerProps>(function BiliVideoPlayer({
  poster,
  streams,
  selectedStreamId,
  danmaku,
  danmakuOn,
  onStreamChange,
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

  function handleKeyboard(event: KeyboardEvent<HTMLElement>) {
    if (event.target instanceof HTMLInputElement || event.target instanceof HTMLSelectElement) return
    if (event.key === ' ' || event.key.toLowerCase() === 'k') {
      event.preventDefault()
      void togglePlayback()
    } else if (event.key.toLowerCase() === 'm') {
      toggleMute()
    } else if (event.key.toLowerCase() === 'f') {
      void toggleFullscreen()
    } else if (event.key === 'ArrowLeft') {
      seek(Math.max(0, currentTime - 5))
    } else if (event.key === 'ArrowRight') {
      seek(Math.min(duration, currentTime + 5))
    }
  }

  const progress = duration > 0 ? Math.min(100, (currentTime / duration) * 100) : 0
  const progressStyle = { '--player-progress': `${progress}%` } as CSSProperties

  return (
    <section ref={shellRef} className={`player-shell ${playing ? 'is-playing' : 'is-paused'} ${fullscreen ? 'is-fullscreen' : ''}`} aria-label="视频播放器" tabIndex={0} onKeyDown={handleKeyboard}>
      {streams.length === 0 && <div className="player-empty"><strong>暂无可播放的视频流</strong><span>视频资源可能仍在处理中</span></div>}
      <video
        ref={assignVideoRef}
        playsInline
        poster={poster}
        onClick={() => void togglePlayback()}
        onLoadedMetadata={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
        onDurationChange={(event) => setDuration(Number.isFinite(event.currentTarget.duration) ? event.currentTarget.duration : 0)}
        onPlaying={(event) => { setPlaying(true); onPlaying(event.currentTarget) }}
        onPause={(event) => { setPlaying(false); onPause(event.currentTarget) }}
        onEnded={() => setPlaying(false)}
        onTimeUpdate={(event) => { setCurrentTime(event.currentTarget.currentTime); onTimeUpdate(event.currentTarget) }}
        onVolumeChange={(event) => { setVolume(event.currentTarget.volume); setMuted(event.currentTarget.muted) }}
      />

      <div className="danmaku-stage" aria-hidden="true">{danmaku.map((item, index) => <span key={`${item.id}-${index}`} style={{ top: `${10 + (index % 7) * 11}%`, color: item.color }}>{item.text}</span>)}</div>

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
          {streams.length > 0 && <label className="player-quality" title="清晰度"><span className="sr-only">清晰度</span><select aria-label="清晰度" value={selectedStreamId} onChange={(event) => onStreamChange(event.target.value)}>{streams.map((stream) => <option key={stream.id} value={stream.id}>{formatStreamLabel(stream)}</option>)}</select></label>}
          <button type="button" aria-label={fullscreen ? '退出全屏' : '全屏'} title={fullscreen ? '退出全屏' : '全屏'} onClick={() => void toggleFullscreen()}><Maximize size={20} /></button>
        </div>
      </div>
    </section>
  )
})

function formatStreamLabel(stream: VideoStream) {
  if (stream.height > 0) return `${stream.height}P`
  if (stream.label && !stream.label.toLowerCase().includes('dash')) return stream.label
  return '清晰'
}
