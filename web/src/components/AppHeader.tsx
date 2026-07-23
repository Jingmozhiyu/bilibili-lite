import { Clock3, Compass, Search, Star, Upload } from 'lucide-react'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import { AuthMenu } from './AuthMenu'

type AppHeaderProps = {
  authOpen: boolean
  onAuthOpenChange: (open: boolean) => void
  onUpload: () => void
}

const leftLinks = ['首页', '番剧', '直播', '游戏中心']

export function AppHeader({ authOpen, onAuthOpenChange, onUpload }: AppHeaderProps) {
  const { session } = useAuth()
  const [query, setQuery] = useState('')
  const location = useLocation()
  const navigate = useNavigate()
  const pageTitle = resolvePageTitle(location.pathname)

  function submitSearch(event: FormEvent) {
    event.preventDefault()
    const value = query.trim().toUpperCase()
    if (/^BV\d+$/.test(value)) navigate(`/video/${value}`)
  }

  return (
    <header className="site-header">
      <nav className="header-inner" aria-label="主导航">
        <div className="header-links">
          {leftLinks.map((label, index) => (
            <Link to="/" key={label}>{index === 0 && <Compass size={17} aria-hidden="true" />}{label}</Link>
          ))}
        </div>
        <form className="header-search" role="search" onSubmit={submitSearch}>
          <input aria-label="搜索 BV 号" placeholder="搜索视频或 BV 号" value={query} onChange={(event) => setQuery(event.target.value)} />
          <button type="submit" aria-label="搜索" title="搜索"><Search size={18} /></button>
        </form>
        <div className="header-actions">
          <AuthMenu open={authOpen} onOpenChange={onAuthOpenChange} onUpload={onUpload} />
          <Link className="header-action-link" to={session ? '/space/me?tab=favorites' : '/'} aria-label="收藏">
            <Star size={19} /><span>收藏</span>
          </Link>
          <Link className="header-action-link" to="/" aria-label="观看历史">
            <Clock3 size={19} /><span>历史</span>
          </Link>
          <button type="button" className="upload-button" onClick={onUpload}><Upload size={18} />投稿</button>
        </div>
      </nav>
      <div className="hero-caption">
        {pageTitle ? (
          <span className="page-hero-title">{pageTitle}</span>
        ) : (
          <Link className="hero-brand" to="/"><span className="brand-mark">b</span><strong>bilibili-lite</strong></Link>
        )}
      </div>
    </header>
  )
}

function resolvePageTitle(pathname: string) {
  if (pathname.startsWith('/video/')) return '视频播放'
  if (pathname.startsWith('/space/')) return '个人空间'
  if (pathname.includes('/history/likes')) return '点赞历史'
  if (pathname.includes('/history/favorites')) return '收藏历史'
  if (pathname.includes('/history/coins')) return '投币历史'
  return ''
}
