import { Coins, Heart, Share2, Star } from 'lucide-react'
import { useState } from 'react'
import type { VideoDetail, VideoEngagement } from '../types'
import { formatCount } from '../utils/format'

type InteractionBarProps = {
  video: VideoDetail
  engagement: VideoEngagement | null
  pending: string
  message: string
  onLike: () => void
  onFavorite: () => void
  onCoin: (targetAmount: number) => void
  onShare: () => void
}

export function InteractionBar({ video, engagement, pending, message, onLike, onFavorite, onCoin, onShare }: InteractionBarProps) {
  const [coinOpen, setCoinOpen] = useState(false)
  const liked = engagement?.liked ?? false
  const favorited = engagement?.favorited ?? false
  const coinAmount = engagement?.myCoinAmount ?? 0

  return (
    <section className="interaction-section" aria-label="视频互动">
      <div className="interaction-actions">
        <button type="button" className={liked ? 'active' : ''} disabled={pending === 'like'} onClick={onLike}>
          <Heart size={22} fill={liked ? 'currentColor' : 'none'} /><span>点赞</span><strong>{formatCount(engagement?.likeCount ?? video.likeCount)}</strong>
        </button>
        <div className="coin-action">
          <button type="button" className={coinAmount > 0 ? 'active' : ''} disabled={pending === 'coin'} onClick={() => setCoinOpen((value) => !value)}>
            <Coins size={22} /><span>投币</span><strong>{formatCount(engagement?.coinCount ?? video.coinCount)}</strong>
          </button>
          {coinOpen && (
            <div className="coin-popover" role="dialog" aria-label="选择投币数量">
              <strong>为这个视频投币</strong>
              <p>已投 {coinAmount}/2 枚，余额 {engagement?.coinBalance ?? 0}</p>
              <div>
                {[1, 2].map((amount) => <button type="button" key={amount} disabled={amount <= coinAmount} onClick={() => { onCoin(amount); setCoinOpen(false) }}>{amount} 枚</button>)}
              </div>
              <small>投币后不可撤销</small>
            </div>
          )}
        </div>
        <button type="button" className={favorited ? 'active' : ''} disabled={pending === 'favorite'} onClick={onFavorite}>
          <Star size={22} fill={favorited ? 'currentColor' : 'none'} /><span>收藏</span><strong>{formatCount(engagement?.favoriteCount ?? video.favoriteCount)}</strong>
        </button>
        <button type="button" disabled={pending === 'share'} onClick={onShare}>
          <Share2 size={22} /><span>分享</span><strong>{formatCount(engagement?.shareCount ?? video.shareCount)}</strong>
        </button>
      </div>
      {message && <p className="interaction-message" role="status">{message}</p>}
    </section>
  )
}
