import { MessageCircle, Play } from 'lucide-react'
import { Link } from 'react-router-dom'
import type { VideoDetail } from '../types'
import { formatCount, formatDuration, formatShortDate } from '../utils/format'

export function VideoCard({ video, historyLabel }: { video: VideoDetail; historyLabel?: string }) {
  return (
    <article className="video-card">
      <Link className="video-cover" to={`/video/${video.bvid}`} aria-label={`播放 ${video.title}`}>
        {video.coverUrl ? (
          <img src={video.coverUrl} alt="" loading="lazy" />
        ) : (
          <span className="cover-placeholder"><strong>b</strong><small>暂无封面</small></span>
        )}
        <span className="cover-metrics">
          <span><Play size={14} fill="currentColor" />{formatCount(video.viewCount)}</span>
          <span><MessageCircle size={14} />{formatCount(video.danmakuCount)}</span>
          <time>{formatDuration(video.durationSeconds)}</time>
        </span>
      </Link>
      <div className="video-card-body">
        <Link className="video-title" to={`/video/${video.bvid}`}>{video.title}</Link>
        <p>{video.ownerName}<span>·</span>{historyLabel || formatShortDate(video.publishTime)}</p>
      </div>
    </article>
  )
}
