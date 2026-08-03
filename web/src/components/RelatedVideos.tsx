import { Link } from 'react-router-dom'
import type { VideoDetail } from '../types'
import { formatCount, formatDuration } from '../utils/format'
import { BiliDanmakuIcon, BiliViewIcon } from './BiliIcons'

export function RelatedVideos({ videos }: { videos: VideoDetail[] }) {
  return (
    <section className="related-videos" aria-labelledby="related-title">
      <h2 id="related-title">相关推荐</h2>
      <div className="related-video-list">
        {videos.slice(0, 8).map((video) => <article className="related-video" key={video.bvid}>
          <Link className="related-cover" to={`/video/${video.bvid}`}>
            {video.coverUrl ? <img src={video.coverUrl} alt="" loading="lazy" /> : <span>暂无封面</span>}
            <time>{formatDuration(video.durationSeconds)}</time>
          </Link>
          <div>
            <Link className="related-title" to={`/video/${video.bvid}`}>{video.title}</Link>
            <p>{video.ownerName}</p>
            <span className="related-metrics"><span><BiliViewIcon size={14} />{formatCount(video.viewCount)}</span><span><BiliDanmakuIcon size={14} />{formatCount(video.danmakuCount)}</span></span>
          </div>
        </article>)}
      </div>
    </section>
  )
}
