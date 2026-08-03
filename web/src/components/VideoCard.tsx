import { Link } from 'react-router-dom'
import type { VideoDetail } from '../types'
import { formatCount, formatDuration, formatShortDate } from '../utils/format'
import { BiliDanmakuIcon, BiliOwnerIcon, BiliViewIcon } from './BiliIcons'

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
          <span><BiliViewIcon size={15} />{formatCount(video.viewCount)}</span>
          <span><BiliDanmakuIcon size={15} />{formatCount(video.danmakuCount)}</span>
          <time>{formatDuration(video.durationSeconds)}</time>
        </span>
      </Link>
      <div className="video-card-body">
        <h3 className="video-title">
          <Link to={`/video/${video.bvid}`}>{video.title}</Link>
        </h3>
        <div className="video-card-meta">
          <BiliOwnerIcon className="video-owner-icon" />
          <span>{video.ownerName}</span>
          <i>·</i>
          <span>{historyLabel || formatShortDate(video.publishTime)}</span>
        </div>
      </div>
    </article>
  )
}
