import { Clock3, Compass, Search, Upload } from 'lucide-react'
import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import { AuthMenu } from './AuthMenu'

type AppHeaderProps = {
  authOpen: boolean
  onAuthOpenChange: (open: boolean) => void
  onUpload: () => void
}

export function AppHeader({ authOpen, onAuthOpenChange, onUpload }: AppHeaderProps) {
  const { session } = useAuth()
  const [query, setQuery] = useState('')
  const navigate = useNavigate()

  function submitSearch(event: FormEvent) {
    event.preventDefault()
    const value = query.trim().toUpperCase()
    if (/^BV\d+$/.test(value)) navigate(`/video/${value}`)
  }

  function requestUpload() {
    onUpload()
  }

  return (
    <header className="site-header">
      <nav className="header-inner" aria-label="主导航">
        <Link className="brand" to="/"><span className="brand-mark">b</span><strong>bilibili-lite</strong></Link>
        <div className="header-links">
          <Link to="/"><Compass size={17} />首页</Link>
          <Link to={session ? '/history/favorites' : '/'}><Clock3 size={17} />我的</Link>
        </div>
        <form className="header-search" role="search" onSubmit={submitSearch}>
          <input aria-label="搜索 BV 号" placeholder="搜索 BV 号" value={query} onChange={(event) => setQuery(event.target.value)} />
          <button type="submit" aria-label="搜索" title="搜索"><Search size={18} /></button>
        </form>
        <div className="header-actions">
          <AuthMenu open={authOpen} onOpenChange={onAuthOpenChange} onUpload={onUpload} />
          <button type="button" className="upload-button" onClick={requestUpload}><Upload size={18} />投稿</button>
        </div>
      </nav>
    </header>
  )
}
