import { Camera, Edit3, Film, House, Search, Sparkles, Trash2, UserRound, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link, Navigate, useParams, useSearchParams } from 'react-router-dom'
import {
  authorizedJson,
  fetchJson,
  normalizeUser,
  normalizeVideoHistory,
  normalizeVideoList,
  normalizeVideoSearch,
  toErrorMessage,
} from '../api'
import { useAuth } from '../auth/useAuth'
import { VideoCard } from '../components/VideoCard'
import type { AuthSession, UserProfile, VideoDetail } from '../types'
import { formatShortDate } from '../utils/format'

type UserTab = 'home' | 'dynamic' | 'submissions'
type HomeSectionKey = 'submissions' | 'favorites' | 'coins' | 'likes'
type HomeVideoItem = { video: VideoDetail; historyLabel?: string }
type HomeSectionData = { items: HomeVideoItem[]; hasMore: boolean; error: string; private: boolean }

const tabs: Array<{ id: UserTab; label: string; icon: typeof Film }> = [
  { id: 'home', label: '主页', icon: House },
  { id: 'dynamic', label: '动态', icon: Sparkles },
  { id: 'submissions', label: '投稿', icon: Film },
]

const homeSectionLabels: Record<HomeSectionKey, string> = {
  submissions: '视频',
  favorites: '收藏',
  coins: '最近投币',
  likes: '最近点赞',
}

export function UserPage() {
  const { userId = '' } = useParams()
  const { session, restoring, setSession } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab') as UserTab | null
  const videoQuery = searchParams.get('q')?.trim() || ''
  const ownPage = userId === 'me' || (!!session && Number(userId) === session.user.id)
  const tab: UserTab = tabs.some((item) => item.id === requestedTab) ? requestedTab! : 'home'
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
          <div className="space-tab-links">
            {tabs.map(({ id, label, icon: Icon }) => (
              <button type="button" key={id} className={tab === id ? 'active' : ''} onClick={() => setSearchParams(id === 'home' ? {} : { tab: id })}>
                <Icon size={17} />{label}
              </button>
            ))}
          </div>
          {tab === 'submissions' && (
            <UserVideoSearch
              key={videoQuery}
              initialValue={videoQuery}
              onSearch={(query) => setSearchParams(query ? { tab: 'submissions', q: query } : { tab: 'submissions' })}
            />
          )}
        </nav>
        {tab === 'home' && <UserHomeContent key={`${profile.id}-${ownPage}`} profile={profile} ownPage={ownPage} />}
        {tab === 'dynamic' && <UserDynamicContent />}
        {tab === 'submissions' && <UserSubmissions key={`${profile.id}-${videoQuery}`} profile={profile} videoQuery={videoQuery} />}
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
          <div>
            <h1>{profile.displayName || profile.username}</h1>
            <img className="profile-level" src={`/levels/level_${profile.level}.svg`} alt={`Lv${profile.level}`} title={`Lv${profile.level} · ${profile.experience} 经验`} />
          </div>
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

function UserVideoSearch({ initialValue, onSearch }: { initialValue: string; onSearch: (query: string) => void }) {
  const [value, setValue] = useState(initialValue)

  return (
    <form className="space-video-search" role="search" onSubmit={(event) => {
      event.preventDefault()
      onSearch(value.trim())
    }}>
      <input aria-label="搜索该用户的视频" value={value} onChange={(event) => setValue(event.target.value)} placeholder="搜索 TA 的视频" />
      <button type="submit" aria-label="搜索该用户的视频" title="搜索"><Search size={16} /></button>
    </form>
  )
}

function UserHomeContent({ profile, ownPage }: { profile: UserProfile; ownPage: boolean }) {
  const { session, setSession } = useAuth()
  const [sections, setSections] = useState<Record<HomeSectionKey, HomeSectionData>>(() => emptyHomeSections(!ownPage))
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true
    let refreshedSession: AuthSession | null = null
    const pageQuery = new URLSearchParams({ page_size: '50' })

    async function loadSubmissions(): Promise<HomeSectionData> {
      try {
        const page = normalizeVideoList(await fetchJson<unknown>(`/api/v1/users/${profile.id}/videos?${pageQuery}`))
        return { items: page.videos.map((video) => ({ video })), hasMore: !!page.nextPageToken, error: '', private: false }
      } catch (loadError) {
        return { items: [], hasMore: false, error: toErrorMessage(loadError, '投稿加载失败'), private: false }
      }
    }

    async function loadPrivateHistory(key: Exclude<HomeSectionKey, 'submissions'>): Promise<HomeSectionData> {
      if (!ownPage || !session) return { items: [], hasMore: false, error: '', private: true }
      try {
        const result = await authorizedJson<unknown>(`/api/v1/users/me/video-${key}?${pageQuery}`, {}, session)
        if (result.session.accessToken !== session.accessToken) refreshedSession = result.session
        const page = normalizeVideoHistory(result.data)
        return {
          items: page.items.map((item) => ({
            video: item.video,
            historyLabel: formatShortDate(item.interactedAt),
          })),
          hasMore: !!page.nextPageToken,
          error: '',
          private: false,
        }
      } catch (loadError) {
        return { items: [], hasMore: false, error: toErrorMessage(loadError, '互动记录加载失败'), private: false }
      }
    }

    void Promise.all([
      loadSubmissions(),
      loadPrivateHistory('favorites'),
      loadPrivateHistory('coins'),
      loadPrivateHistory('likes'),
    ]).then(([submissions, favorites, coins, likes]) => {
      if (!active) return
      setSections({ submissions, favorites, coins, likes })
      if (refreshedSession) setSession(refreshedSession)
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [ownPage, profile.id, session, setSession])

  if (loading) {
    return <div className="space-home-sections">{Object.keys(homeSectionLabels).map((key) => <HomeSectionSkeleton key={key} label={homeSectionLabels[key as HomeSectionKey]} />)}</div>
  }

  return (
    <div className="space-home-sections" aria-live="polite">
      {(Object.keys(homeSectionLabels) as HomeSectionKey[]).map((key) => <HomeVideoSection key={key} sectionKey={key} data={sections[key]} />)}
    </div>
  )
}

function HomeVideoSection({ sectionKey, data }: { sectionKey: HomeSectionKey; data: HomeSectionData }) {
  const count = data.private ? '仅本人可见' : `${data.items.length}${data.hasMore ? '+' : ''}`
  return <section className="space-home-section">
    <header><h2>{homeSectionLabels[sectionKey]}<span>·</span><b>{count}</b></h2></header>
    {data.private ? <p className="space-section-empty">该分区仅对用户本人显示。</p>
      : data.error ? <p className="inline-error" role="status">{data.error}</p>
        : data.items.length > 0 ? <div className="space-content-grid">{data.items.slice(0, 5).map((item) => <VideoCard key={`${sectionKey}-${item.video.bvid}`} video={item.video} historyLabel={item.historyLabel} />)}</div>
          : <p className="space-section-empty">这里还没有视频。</p>}
  </section>
}

function HomeSectionSkeleton({ label }: { label: string }) {
  return <section className="space-home-section"><header><h2>{label}<span>·</span><b>--</b></h2></header><div className="space-content-grid">{Array.from({ length: 5 }, (_, index) => <div className="video-skeleton" key={index} />)}</div></section>
}

function emptyHomeSections(privateSections: boolean): Record<HomeSectionKey, HomeSectionData> {
  const empty = (privateSection: boolean): HomeSectionData => ({ items: [], hasMore: false, error: '', private: privateSection })
  return { submissions: empty(false), favorites: empty(privateSections), coins: empty(privateSections), likes: empty(privateSections) }
}

function UserDynamicContent() {
  return <section className="space-empty"><Sparkles size={30} /><h2>还没有动态</h2><p>发布动态后会显示在这里。</p></section>
}

function UserSubmissions({ profile, videoQuery }: { profile: UserProfile; videoQuery: string }) {
  const [videos, setVideos] = useState<VideoDetail[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')

  async function load(token: string) {
    const query = new URLSearchParams({ page_size: '20' })
    if (token) query.set('page_token', token)
    if (videoQuery) {
      query.set('query', videoQuery)
      query.set('owner_id', String(profile.id))
      query.set('order', '1')
      return normalizeVideoSearch(await fetchJson<unknown>(`/api/v1/search/videos?${query}`))
    }
    return normalizeVideoList(await fetchJson<unknown>(`/api/v1/users/${profile.id}/videos?${query}`))
  }

  useEffect(() => {
    let active = true
    void load('').then((page) => {
      if (!active) return
      setVideos(page.videos)
      setNextPageToken(page.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '投稿加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
    // The component is keyed by profile and query.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function loadMore() {
    if (!nextPageToken || loadingMore) return
    setLoadingMore(true)
    try {
      const page = await load(nextPageToken)
      setVideos((current) => [...current, ...page.videos])
      setNextPageToken(page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页加载失败'))
    } finally {
      setLoadingMore(false)
    }
  }

  if (loading) return <div className="space-content-grid">{Array.from({ length: 10 }, (_, index) => <div className="video-skeleton" key={index} />)}</div>
  return <section className="space-content" aria-live="polite">
    {error && <p className="inline-error" role="status">{error}</p>}
    {videos.length > 0 ? <div className="space-content-grid">{videos.map((video) => <VideoCard key={video.bvid} video={video} />)}</div>
      : <div className="space-empty"><UserRound size={30} /><h2>{videoQuery ? `没有找到“${videoQuery}”` : '这里还没有投稿'}</h2><p>{videoQuery ? '换个关键词搜索该用户的视频。' : '发布的视频会出现在这里。'}</p></div>}
    {nextPageToken && <button type="button" className="load-more-button" disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? '加载中' : '加载更多'}</button>}
  </section>
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
