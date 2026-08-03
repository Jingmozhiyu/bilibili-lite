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
  commentCount: MetricValue
  publishTime?: string
  tags: string[]
  ownerId?: number
  status?: string
  reviewReason?: string
  submittedAt?: string
  reviewedAt?: string
}

export type VideoListPage = {
  videos: VideoDetail[]
  nextPageToken: string
}

export type VideoSearchPage = VideoListPage & {
  totalHits: number
  processingTimeMs: number
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
  id: number
  userId: number
  userName: string
  timeSeconds: number
  text: string
  color: string
  createdAt?: string
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
  coinBalance: number
  isAdmin: boolean
  experience: number
  level: number
}

export type UserProfile = AuthUser

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

export type VideoEngagement = {
  bvid: string
  liked: boolean
  favorited: boolean
  myCoinAmount: number
  likeCount: MetricValue
  favoriteCount: MetricValue
  coinCount: MetricValue
  shareCount: MetricValue
  coinBalance: number
}

export type VideoHistoryItem = {
  video: VideoDetail
  interactedAt?: string
  coinAmount: number
}

export type VideoHistoryPage = {
  items: VideoHistoryItem[]
  nextPageToken: string
}

export type VideoComment = {
  id: number
  bvid: string
  userId: number
  userName: string
  userAvatarUrl?: string
  content: string
  createdAt?: string
  rootId: number
  parentId: number
  replyToUserId: number
  replyToUserName: string
  likeCount: MetricValue
  liked: boolean
  replyCount: MetricValue
  deleted: boolean
}

export type VideoCommentInteraction = {
  commentId: number
  liked: boolean
  likeCount: MetricValue
}

export type VideoCommentHistoryItem = {
  video: VideoDetail
  comment: VideoComment
}

export type VideoCommentHistoryPage = {
  items: VideoCommentHistoryItem[]
  nextPageToken: string
}

export type VideoCommentPage = {
  comments: VideoComment[]
  nextPageToken: string
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

export type UploadResult = {
  bvid: string
  status: string
  manifestUrl: string
  coverUrl: string
  videoUrl: string
}
