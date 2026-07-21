export type MetricValue = number | string

export type VideoDetail = {
  bvid: string
  title: string
  description: string
  ownerName: string
  ownerAvatarUrl?: string
  coverUrl?: string
  durationSeconds: MetricValue
  viewCount: MetricValue
  danmakuCount: MetricValue
  likeCount: MetricValue
  coinCount: MetricValue
  favoriteCount: MetricValue
  shareCount: MetricValue
  publishTime?: string
  tags: string[]
  ownerId?: number
  status?: string
}

export type VideoStream = {
  id: string
  label: string
  codec: string
  mimeType: string
  url: string
  width: number
  height: number
  bandwidth: number
  defaultStream: boolean
}

export type DanmakuItem = {
  timeSeconds: number
  text: string
  color: string
}

export type VideoPlay = {
  bvid: string
  streams: VideoStream[]
  danmaku?: {
    enabled: boolean
    format: string
    items: DanmakuItem[]
  }
}

export type AuthUser = {
  id: number
  username: string
  displayName: string
  avatarUrl?: string
  bio?: string
}

export type AuthSession = {
  accessToken: string
  refreshToken: string
  expiresAt: string
  refreshExpiresAt: string
  user: AuthUser
}

export type VideoLike = {
  bvid: string
  liked: boolean
  likeCount: MetricValue
}

export type VideoViewSession = {
  sessionId: string
  startedAt: string
}

export type VideoViewResult = {
  counted: boolean
  viewCount: MetricValue
  remainingToday: number
  nextEligibleAt: string
}
