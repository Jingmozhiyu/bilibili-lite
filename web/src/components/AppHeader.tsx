import { Download, Search, Upload } from 'lucide-react'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { AuthMenu } from './AuthMenu'
import { BiliHomeIcon } from './BiliIcons'
import { HeaderHistoryPreview } from './HeaderHistoryPreview'

type AppHeaderProps = {
  authOpen: boolean
  onAuthOpenChange: (open: boolean) => void
  onUpload: () => void
}

const leftLinks = ['首页', '番剧', '直播', '游戏中心', '会员购', '漫画', '赛事']

export function AppHeader({ authOpen, onAuthOpenChange, onUpload }: AppHeaderProps) {
  const location = useLocation()
  const navigate = useNavigate()
  const isHome = location.pathname === '/'
  const isSpace = location.pathname.startsWith('/space/')
  const isSearch = location.pathname === '/search'
  const isVideo = location.pathname.startsWith('/video/')
  const isSolid = !isHome && !isSpace
  const [activePreview, setActivePreview] = useState<'favorites' | 'views' | null>(null)
  const routeKeyword = location.pathname === '/search' ? new URLSearchParams(location.search).get('keyword') || '' : ''
  const openAuth = () => {
    setActivePreview(null)
    onAuthOpenChange(true)
  }

  return (
    <header className={`site-header ${isHome ? 'home-header' : ''} ${isSpace ? 'space-header' : ''} ${isSolid ? 'solid-header' : ''} ${isSearch ? 'search-header' : ''} ${isVideo ? 'video-header' : ''}`}>
      <div className="header-bg-shadow" aria-hidden="true"><span className="top" />{isSpace && <span className="bottom" />}</div>
      <nav className="header-inner" aria-label="主导航">
        <div className="header-links">
          {!isHome && <Link className="compact-header-brand" to="/" aria-label="返回首页"><span className="bilibili-blue-logo" aria-hidden="true" /></Link>}
          {leftLinks.map((label, index) => (
            <Link to="/" key={label}>{index === 0 && <BiliHomeIcon size={18} />}{label}</Link>
          ))}
          <Link to="/"><Download size={17} aria-hidden="true" />下载客户端</Link>
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
      {isHome && <div className="hero-caption"><Link className="hero-logo" to="/" aria-label="返回首页"><img src="/bilibili-logo.png" alt="bilibili" /></Link></div>}
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
