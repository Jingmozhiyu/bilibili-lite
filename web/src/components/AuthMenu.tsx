import { Coins, History, LogOut, ShieldCheck, Upload, UserRound } from 'lucide-react'
import { useRef, useState } from 'react'
import type { FocusEvent } from 'react'
import { Link } from 'react-router-dom'
import { authorizedFetch, normalizeAuthSession, postJson, toErrorMessage } from '../api'
import { useAuth } from '../auth/useAuth'

type AuthMenuProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpload: () => void
}

const hoverOpenDelay = 240
const hoverCloseDelay = 280

export function AuthMenu({ open, onOpenChange, onUpload }: AuthMenuProps) {
  const { session, restoring, setSession } = useAuth()
  const [username, setUsername] = useState('demo')
  const [password, setPassword] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const openTimer = useRef<number | null>(null)
  const closeTimer = useRef<number | null>(null)
  const userLabel = session?.user.displayName || session?.user.username || '用户'

  function cancelOpen() {
    if (openTimer.current !== null) {
      window.clearTimeout(openTimer.current)
      openTimer.current = null
    }
  }

  function cancelClose() {
    if (closeTimer.current !== null) {
      window.clearTimeout(closeTimer.current)
      closeTimer.current = null
    }
  }

  function openAccountMenu() {
    if (!session) return
    cancelOpen()
    cancelClose()
    onOpenChange(true)
  }

  function scheduleOpen() {
    if (!session || open) return
    cancelOpen()
    cancelClose()
    openTimer.current = window.setTimeout(openAccountMenu, hoverOpenDelay)
  }

  function scheduleClose() {
    if (!session) return
    cancelOpen()
    cancelClose()
    closeTimer.current = window.setTimeout(() => onOpenChange(false), hoverCloseDelay)
  }

  function handleBlur(event: FocusEvent<HTMLDivElement>) {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) scheduleClose()
  }

  async function login(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setPending(true)
    setError('')
    try {
      const response = await postJson<unknown>('/api/v1/auth/login', { username, password })
      setSession(normalizeAuthSession(response))
      setPassword('')
    } catch (loginError) {
      const message = toErrorMessage(loginError, '')
      setError(message.startsWith('401') ? '用户名或密码不正确' : '登录失败，请稍后重试')
    } finally {
      setPending(false)
    }
  }

  async function logout() {
    if (!session) return
    setPending(true)
    try {
      await authorizedFetch('/api/v1/auth/logout', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' }, session)
    } catch {
      // Stateless logout is completed locally even when the server is unavailable.
    } finally {
      setSession(null)
      onOpenChange(false)
      setPending(false)
    }
  }

  return (
    <div className="auth-menu" onMouseEnter={scheduleOpen} onMouseLeave={scheduleClose} onFocus={openAccountMenu} onBlur={handleBlur}>
      {session ? (
        <Link
          className="avatar-button signed-in"
          to="/space/me"
          aria-label={`进入 ${userLabel} 的个人主页`}
          aria-expanded={open}
          onClick={() => onOpenChange(false)}
        >
          {session.user.avatarUrl ? <img src={session.user.avatarUrl} alt="" /> : userLabel.slice(0, 1).toUpperCase()}
        </Link>
      ) : (
        <button
          type="button"
          className="avatar-button"
          aria-label="登录"
          aria-expanded={open}
          onClick={() => {
            onOpenChange(!open)
            setError('')
          }}
        >
          {restoring ? '…' : '登录'}
        </button>
      )}

      {open && (
        <div
          className="account-popover"
          role="dialog"
          aria-label={session ? '账户菜单' : '登录'}
          onMouseEnter={cancelClose}
          onMouseLeave={scheduleClose}
        >
          {session ? (
            <div className="account-panel">
              <div className="account-profile">
                <span className="account-avatar">
                  {session.user.avatarUrl ? <img src={session.user.avatarUrl} alt="" /> : userLabel.slice(0, 1).toUpperCase()}
                </span>
                <div>
                  <span className="account-name-row"><strong>{userLabel}</strong>{session.user.isAdmin && <em>管理员</em>}</span>
                  <small>@{session.user.username}</small>
                </div>
              </div>
              <div className="coin-balance"><Coins size={17} /><span>硬币余额</span><strong>{session.user.coinBalance}</strong></div>
              <nav className="account-links" aria-label="互动历史">
                <Link to="/space/me" onClick={() => onOpenChange(false)}><UserRound size={17} />个人主页</Link>
                <Link to="/space/me?tab=likes" onClick={() => onOpenChange(false)}><History size={17} />点赞历史</Link>
                <Link to="/space/me?tab=favorites" onClick={() => onOpenChange(false)}><History size={17} />收藏历史</Link>
                <Link to="/space/me?tab=coins" onClick={() => onOpenChange(false)}><History size={17} />投币历史</Link>
                <Link to="/history/views" onClick={() => onOpenChange(false)}><History size={17} />观看历史</Link>
                {session.user.isAdmin && <Link to="/admin/reviews" onClick={() => onOpenChange(false)}><ShieldCheck size={17} />内容审核</Link>}
              </nav>
              <button type="button" className="menu-primary" onClick={() => { onUpload(); onOpenChange(false) }}>
                <Upload size={17} />投稿视频
              </button>
              <button type="button" className="menu-quiet" disabled={pending} onClick={() => void logout()}>
                <LogOut size={17} />{pending ? '正在退出' : '退出登录'}
              </button>
            </div>
          ) : (
            <form className="login-form" onSubmit={login}>
              <div><strong>登录 bilibili-lite</strong><p>互动、评论和投稿需要登录</p></div>
              <label><span>用户名</span><input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /></label>
              <label><span>密码</span><input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required autoFocus /></label>
              {error && <p className="form-error" role="alert">{error}</p>}
              <button className="menu-primary" type="submit" disabled={pending}>{pending ? '登录中' : '登录'}</button>
            </form>
          )}
        </div>
      )}
    </div>
  )
}
