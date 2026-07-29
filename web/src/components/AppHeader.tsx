import { Compass, Search, Upload } from 'lucide-react'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { AuthMenu } from './AuthMenu'
import { HeaderHistoryPreview } from './HeaderHistoryPreview'

type AppHeaderProps = {
  authOpen: boolean
  onAuthOpenChange: (open: boolean) => void
  onUpload: () => void
}

const leftLinks = ['首页', '番剧', '直播', '游戏中心']

export function AppHeader({ authOpen, onAuthOpenChange, onUpload }: AppHeaderProps) {
  const location = useLocation()
  const navigate = useNavigate()
  const pageTitle = resolvePageTitle(location.pathname)
  const isHome = location.pathname === '/'
  const isSpace = location.pathname.startsWith('/space/')
  const isSearch = location.pathname === '/search'
  const [activePreview, setActivePreview] = useState<'favorites' | 'views' | null>(null)
  const routeKeyword = location.pathname === '/search' ? new URLSearchParams(location.search).get('keyword') || '' : ''
  const openAuth = () => {
    setActivePreview(null)
    onAuthOpenChange(true)
  }

  return (
    <header className={`site-header ${isHome ? 'home-header' : ''} ${isSpace ? 'space-header' : ''} ${isSearch ? 'search-header' : ''}`}>
      <div className="header-bg-shadow" aria-hidden="true"><span className="top" />{isSpace && <span className="bottom" />}</div>
      <nav className="header-inner" aria-label="主导航">
        <div className="header-links">
          {isSearch && <Link className="compact-header-brand" to="/"><span className="brand-mark">b</span><strong>bilibili-lite</strong></Link>}
          {leftLinks.map((label, index) => (
            <Link to="/" key={label}>{index === 0 && <Compass size={17} aria-hidden="true" />}{label}</Link>
          ))}
        </div>
        {!isSearch && <HeaderSearch key={routeKeyword} initialQuery={routeKeyword} onSearch={(value) => navigate(`/search?keyword=${encodeURIComponent(value)}`)} />}
        <div className="header-actions">
          <AuthMenu
            open={authOpen}
            onOpenChange={(open) => {
              onAuthOpenChange(open)
              if (open) setActivePreview(null)
            }}
            onUpload={onUpload}
          />
          <HeaderHistoryPreview
            kind="favorites"
            open={activePreview === 'favorites'}
            onOpenChange={(open) => {
              setActivePreview((current) => open ? 'favorites' : current === 'favorites' ? null : current)
              if (open) onAuthOpenChange(false)
            }}
            onLoginRequired={openAuth}
          />
          <HeaderHistoryPreview
            kind="views"
            open={activePreview === 'views'}
            onOpenChange={(open) => {
              setActivePreview((current) => open ? 'views' : current === 'views' ? null : current)
              if (open) onAuthOpenChange(false)
            }}
            onLoginRequired={openAuth}
          />
          <button type="button" className="upload-button" onClick={onUpload}><Upload size={18} />投稿</button>
        </div>
      </nav>
      {!isSpace && !isSearch && <div className="hero-caption">
        {pageTitle ? (
          <span className="page-hero-title">{pageTitle}</span>
        ) : (
          <Link className="hero-logo" to="/" aria-label="返回首页">
            <img src="/bilibili-logo.png" alt="bilibili" />
          </Link>
        )}
      </div>}
    </header>
  )
}

function HeaderSearch({ initialQuery, onSearch }: { initialQuery: string; onSearch: (query: string) => void }) {
  const [query, setQuery] = useState(initialQuery)

  function submitSearch(event: FormEvent) {
    event.preventDefault()
    const value = query.trim()
    if (value) onSearch(value)
  }

  return (
    <form className="header-search" role="search" onSubmit={submitSearch}>
      <input aria-label="搜索视频" placeholder="搜索视频、作者或 BV 号" value={query} onChange={(event) => setQuery(event.target.value)} />
      <button type="submit" aria-label="搜索" title="搜索"><Search size={18} /></button>
    </form>
  )
}

function resolvePageTitle(pathname: string) {
  if (pathname.startsWith('/video/')) return '视频播放'
  if (pathname.startsWith('/space/')) return '个人空间'
  if (pathname.startsWith('/search')) return '搜索'
  if (pathname.startsWith('/admin/reviews')) return '内容管理'
  if (pathname.includes('/history/views')) return '观看历史'
  if (pathname.includes('/history/likes')) return '点赞历史'
  if (pathname.includes('/history/favorites')) return '收藏历史'
  if (pathname.includes('/history/coins')) return '投币历史'
  return ''
}
