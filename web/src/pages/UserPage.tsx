import { Camera, Coins, Edit3, Film, Heart, MessageCircle, Star, Trash2, UserRound, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, Navigate, useParams, useSearchParams } from 'react-router-dom'
import {
  authorizedJson,
  fetchJson,
  normalizeUser,
  normalizeVideoCommentHistory,
  normalizeVideoHistory,
  normalizeVideoList,
  toErrorMessage,
} from '../api'
import { useAuth } from '../auth/useAuth'
import { VideoCard } from '../components/VideoCard'
import type { UserProfile, VideoCommentHistoryItem, VideoHistoryItem, VideoDetail } from '../types'
import { formatDate } from '../utils/format'

type UserTab = 'submissions' | 'likes' | 'favorites' | 'coins' | 'comments'

const tabs: Array<{ id: UserTab; label: string; icon: typeof Film; private?: boolean }> = [
  { id: 'submissions', label: '投稿', icon: Film },
  { id: 'likes', label: '点赞', icon: Heart, private: true },
  { id: 'favorites', label: '收藏', icon: Star, private: true },
  { id: 'coins', label: '投币', icon: Coins, private: true },
  { id: 'comments', label: '评论', icon: MessageCircle, private: true },
]

export function UserPage() {
  const { userId = '' } = useParams()
  const { session, restoring, setSession } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab') as UserTab | null
  const ownPage = userId === 'me' || (!!session && Number(userId) === session.user.id)
  const tab: UserTab = tabs.some((item) => item.id === requestedTab && (!item.private || ownPage)) ? requestedTab! : 'submissions'
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [profileLoading, setProfileLoading] = useState(true)
  const [profileError, setProfileError] = useState('')

  useEffect(() => {
    if (restoring) return
    if (userId === 'me' && !session) return
    let active = true
    const request = ownPage && session
      ? authorizedJson<unknown>('/api/v1/users/me', {}, session).then((result) => {
          if (result.session.accessToken !== session.accessToken) setSession(result.session)
          return result.data
        })
      : fetchJson<unknown>(`/api/v1/users/${encodeURIComponent(userId)}`)
    void request.then((payload) => {
      if (active) setProfile(normalizeUser(payload))
    }).catch((error) => {
      if (active) setProfileError(toErrorMessage(error, '用户资料加载失败'))
    }).finally(() => {
      if (active) setProfileLoading(false)
    })
    return () => { active = false }
  }, [ownPage, restoring, session, setSession, userId])

  if (!restoring && userId === 'me' && !session) return <Navigate to="/" replace />
  if (profileLoading || restoring) return <UserPageSkeleton />
  if (!profile) return <UserPageError message={profileError || '用户不存在'} />

  return (
    <main className="space-page">
      <ProfileHeader profile={profile} ownPage={ownPage} onUpdated={(next) => {
        setProfile(next)
        if (session) setSession({ ...session, user: next })
      }} />
      <div className="space-body">
        <nav className="space-tabs" aria-label="用户内容">
          {tabs.filter((item) => !item.private || ownPage).map(({ id, label, icon: Icon }) => (
            <button type="button" key={id} className={tab === id ? 'active' : ''} onClick={() => setSearchParams(id === 'submissions' ? {} : { tab: id })}>
              <Icon size={17} />{label}
            </button>
          ))}
        </nav>
        <UserTabContent key={`${profile.id}-${tab}`} profile={profile} tab={tab} />
      </div>
    </main>
  )
}

function ProfileHeader({ profile, ownPage, onUpdated }: { profile: UserProfile; ownPage: boolean; onUpdated: (profile: UserProfile) => void }) {
  const { session, setSession } = useAuth()
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState({ displayName: profile.displayName, bio: profile.bio || '' })
  const [pending, setPending] = useState(false)
  const [avatarPending, setAvatarPending] = useState(false)
  const [error, setError] = useState('')
  const avatarInputRef = useRef<HTMLInputElement>(null)

  function openEditor() {
    setForm({ displayName: profile.displayName, bio: profile.bio || '' })
    setError('')
    setEditing(true)
  }

  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!session) return
    setPending(true)
    setError('')
    try {
      const result = await authorizedJson<unknown>('/api/v1/users/me', {
        method: 'PATCH', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ display_name: form.displayName, bio: form.bio }),
      }, session)
      const next = normalizeUser(result.data)
      setSession({ ...result.session, user: next })
      onUpdated(next)
      setEditing(false)
    } catch (saveError) {
      setError(toErrorMessage(saveError, '资料保存失败'))
    } finally {
      setPending(false)
    }
  }

  async function uploadAvatar(file: File) {
    if (!session) return
    const validExtension = /\.(jpe?g|png)$/i.test(file.name)
    const validType = file.type === '' || file.type === 'image/jpeg' || file.type === 'image/png'
    if (!validExtension || !validType) {
      setError('头像仅支持 JPG 或 PNG 图片')
      if (avatarInputRef.current) avatarInputRef.current.value = ''
      return
    }
    if (file.size > 10 * 1024 * 1024) {
      setError('头像不能超过 10 MB')
      if (avatarInputRef.current) avatarInputRef.current.value = ''
      return
    }
    setAvatarPending(true)
    setError('')
    try {
      const result = await authorizedJson<unknown>('/api/v1/users/me/avatar', {
        method: 'PUT', headers: { 'Content-Type': file.type }, body: file,
      }, session)
      const next = normalizeUser(result.data)
      setSession({ ...result.session, user: next })
      onUpdated(next)
    } catch (uploadError) {
      setError(toErrorMessage(uploadError, '头像上传失败'))
    } finally {
      setAvatarPending(false)
      if (avatarInputRef.current) avatarInputRef.current.value = ''
    }
  }

  async function removeAvatar() {
    if (!session || !profile.avatarUrl) return
    setAvatarPending(true)
    setError('')
    try {
      const result = await authorizedJson<unknown>('/api/v1/users/me/avatar', { method: 'DELETE' }, session)
      const next = normalizeUser(result.data)
      setSession({ ...result.session, user: next })
      onUpdated(next)
    } catch (removeError) {
      setError(toErrorMessage(removeError, '头像移除失败'))
    } finally {
      setAvatarPending(false)
    }
  }

  return (
    <section className="profile-band">
      <div className="profile-inner">
        <Avatar profile={profile} size="large" />
        <div className="profile-copy">
          <div><h1>{profile.displayName || profile.username}</h1><span>@{profile.username}</span></div>
          <p>{profile.bio || '这个人还没有写个人简介。'}</p>
        </div>
        {ownPage && <button type="button" className="profile-edit-button" onClick={openEditor}><Edit3 size={17} />编辑资料</button>}
      </div>
      {editing && (
        <div className="profile-editor-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setEditing(false) }}>
          <form className="profile-editor" onSubmit={save}>
            <header><div><strong>编辑资料</strong><p>这些信息会显示在个人主页和视频作者栏。</p></div><button type="button" className="icon-button" aria-label="关闭" title="关闭" onClick={() => setEditing(false)}><X size={19} /></button></header>
            <div className="avatar-editor">
              <Avatar profile={profile} size="large" />
              <div>
                <strong>个人头像</strong>
                <p>支持 JPG、PNG，文件不超过 10 MB</p>
                <div className="avatar-editor-actions">
                  <label className="secondary-button" htmlFor="avatar-upload"><Camera size={16} />{avatarPending ? '处理中' : '更换头像'}</label>
                  {profile.avatarUrl && <button type="button" className="avatar-remove-button" disabled={avatarPending} onClick={() => void removeAvatar()}><Trash2 size={16} />恢复默认</button>}
                </div>
                <input ref={avatarInputRef} id="avatar-upload" className="visually-hidden-input" type="file" disabled={avatarPending} accept="image/jpeg,image/png,.jpg,.jpeg,.png" onChange={(event) => { const file = event.target.files?.[0]; if (file) void uploadAvatar(file) }} />
              </div>
            </div>
            <label><span>昵称</span><input maxLength={100} required value={form.displayName} onChange={(event) => setForm({ ...form, displayName: event.target.value })} /></label>
            <label><span>个人简介</span><textarea maxLength={500} rows={4} value={form.bio} onChange={(event) => setForm({ ...form, bio: event.target.value })} /></label>
            {error && <p className="form-error" role="alert">{error}</p>}
            <footer><button type="button" className="secondary-button" onClick={() => setEditing(false)}>取消</button><button type="submit" className="primary-button" disabled={pending}>{pending ? '保存中' : '保存'}</button></footer>
          </form>
        </div>
      )}
    </section>
  )
}

function UserTabContent({ profile, tab }: { profile: UserProfile; tab: UserTab }) {
  const { session, setSession } = useAuth()
  const [videos, setVideos] = useState<VideoDetail[]>([])
  const [history, setHistory] = useState<VideoHistoryItem[]>([])
  const [comments, setComments] = useState<VideoCommentHistoryItem[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')

  const endpoint = useMemo(() => {
    if (tab === 'submissions') return `/api/v1/users/${profile.id}/videos`
    if (tab === 'comments') return '/api/v1/users/me/video-comments'
    return `/api/v1/users/me/video-${tab}`
  }, [profile.id, tab])

  async function load(token: string) {
    const query = new URLSearchParams({ page_size: '12' })
    if (token) query.set('page_token', token)
    if (tab === 'submissions') return { kind: 'videos' as const, page: normalizeVideoList(await fetchJson<unknown>(`${endpoint}?${query}`)) }
    if (!session) throw new Error('请先登录')
    const result = await authorizedJson<unknown>(`${endpoint}?${query}`, {}, session)
    setSession(result.session)
    if (tab === 'comments') return { kind: 'comments' as const, page: normalizeVideoCommentHistory(result.data) }
    return { kind: 'history' as const, page: normalizeVideoHistory(result.data) }
  }

  useEffect(() => {
    let active = true
    void load('').then((result) => {
      if (!active) return
      if (result.kind === 'videos') setVideos(result.page.videos)
      if (result.kind === 'history') setHistory(result.page.items)
      if (result.kind === 'comments') setComments(result.page.items)
      setNextPageToken(result.page.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '内容加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
    // The tab component is keyed by profile and tab; avoid reloading after token rotation.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [endpoint, tab])

  async function loadMore() {
    if (!nextPageToken || loadingMore) return
    setLoadingMore(true)
    try {
      const result = await load(nextPageToken)
      if (result.kind === 'videos') setVideos((current) => [...current, ...result.page.videos])
      if (result.kind === 'history') setHistory((current) => [...current, ...result.page.items])
      if (result.kind === 'comments') setComments((current) => [...current, ...result.page.items])
      setNextPageToken(result.page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页加载失败'))
    } finally {
      setLoadingMore(false)
    }
  }

  if (loading) return <div className="space-content-grid">{Array.from({ length: 8 }, (_, index) => <div className="video-skeleton" key={index} />)}</div>
  const empty = tab === 'submissions' ? videos.length === 0 : tab === 'comments' ? comments.length === 0 : history.length === 0
  return (
    <section className="space-content" aria-live="polite">
      {error && <p className="inline-error" role="status">{error}</p>}
      {tab === 'submissions' && videos.length > 0 && <div className="space-content-grid">{videos.map((video) => <VideoCard key={video.bvid} video={video} />)}</div>}
      {tab !== 'submissions' && tab !== 'comments' && history.length > 0 && <div className="space-content-grid">{history.map((item) => <VideoCard key={`${item.video.bvid}-${item.interactedAt}`} video={item.video} historyLabel={`${tab === 'coins' ? `${item.coinAmount} 币 · ` : ''}${formatDate(item.interactedAt)}`} />)}</div>}
      {tab === 'comments' && comments.length > 0 && <div className="comment-history-list">{comments.map((item) => <Link key={item.comment.id} to={`/video/${item.video.bvid}`}><div><strong>{item.video.title}</strong><time>{formatDate(item.comment.createdAt)}</time></div><p>{item.comment.replyToUserName && <span>回复 {item.comment.replyToUserName}：</span>}{item.comment.content}</p></Link>)}</div>}
      {empty && <div className="space-empty"><UserRound size={30} /><h2>这里还没有内容</h2><p>{tab === 'submissions' ? '发布的视频会出现在这里。' : '完成对应互动后，历史记录会出现在这里。'}</p></div>}
      {nextPageToken && <button type="button" className="load-more-button" disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? '加载中' : '加载更多'}</button>}
    </section>
  )
}

function Avatar({ profile, size }: { profile: UserProfile; size?: 'large' }) {
  const label = (profile.displayName || profile.username).slice(0, 1).toUpperCase()
  return <span className={`profile-avatar ${size === 'large' ? 'large' : ''}`}>{profile.avatarUrl ? <img src={profile.avatarUrl} alt="" /> : label}</span>
}

function UserPageSkeleton() {
  return <main className="space-page"><div className="profile-band profile-skeleton" /><div className="space-body"><div className="space-content-grid">{Array.from({ length: 8 }, (_, index) => <div className="video-skeleton" key={index} />)}</div></div></main>
}

function UserPageError({ message }: { message: string }) {
  return <main className="error-page"><UserRound size={34} /><h1>用户主页加载失败</h1><p>{message}</p><Link to="/">返回首页</Link></main>
}
