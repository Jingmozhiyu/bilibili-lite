import { Coins, History, LogOut, ShieldCheck, Upload, UserRound } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { FocusEvent } from 'react'
import { Link } from 'react-router-dom'
import { authorizedFetch, authorizedJson, normalizeAuthSession, normalizeUser, postJson, toErrorMessage } from '../api'
import { useAuth } from '../auth/useAuth'
import type { AuthUser } from '../types'

type AuthMenuProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpload: () => void
}

const hoverOpenDelay = 240
const hoverCloseDelay = 280
const levelThresholds = [0, 10, 50, 150, 450, 1080, 2880] as const

export function AuthMenu({ open, onOpenChange, onUpload }: AuthMenuProps) {
  const { session, restoring, setSession } = useAuth()
  const [authMode, setAuthMode] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const openTimer = useRef<number | null>(null)
  const closeTimer = useRef<number | null>(null)
  const userLabel = session?.user.displayName || session?.user.username || '用户'
  const level = Math.min(6, Math.max(0, session?.user.level || 0))
  const experience = Math.max(0, session?.user.experience || 0)
  const nextLevelExperience = levelThresholds[Math.min(level + 1, 6)]
  const levelStartExperience = levelThresholds[level]
  const levelProgress = level === 6
    ? 100
    : Math.min(100, Math.max(0, ((experience - levelStartExperience) / (nextLevelExperience - levelStartExperience)) * 100))

  useEffect(() => {
    if (!open || !session) return
    let active = true
    void authorizedJson<unknown>('/api/v1/users/me', {}, session).then((result) => {
      if (!active) return
      const user = normalizeUser(result.data)
      if (result.session.accessToken !== session.accessToken || !sameUser(user, session.user)) {
        setSession({ ...result.session, user })
      }
    }).catch(() => {
      // Keep the cached account summary usable when refreshing it fails.
    })
    return () => { active = false }
  }, [open, session, setSession])

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

  async function submitAuthentication(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setPending(true)
    setError('')
    try {
      const registering = authMode === 'register'
      const response = await postJson<unknown>(registering ? '/api/v1/auth/register' : '/api/v1/auth/login', registering ? { username, password, displayName } : { username, password })
      setSession(normalizeAuthSession(response))
      setPassword('')
      setDisplayName('')
    } catch (loginError) {
      const message = toErrorMessage(loginError, '')
      if (message.startsWith('401')) setError('用户名或密码不正确')
      else if (message.startsWith('409')) setError('该用户名已经被注册')
      else if (message.startsWith('429')) setError('请求过于频繁，请稍后再试')
      else if (message.startsWith('400') && authMode === 'register') setError('用户名需为3–32位字母、数字、下划线或连字符，密码至少8位')
      else setError(authMode === 'register' ? '注册失败，请稍后重试' : '登录失败，请稍后重试')
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
                  <span className="account-name-row">
                    <strong>{userLabel}</strong>
                    <img className="account-level" src={`/levels/level_${level}.svg`} alt={`Lv${level}`} />
                    {session.user.isAdmin && <em>管理员</em>}
                  </span>
                  <span className="account-experience">
                    {level === 6 ? `${experience} 经验 · 已满级` : `${experience} / ${nextLevelExperience} 经验`}
                  </span>
                  <span className="account-experience-track" aria-hidden="true"><i style={{ width: `${levelProgress}%` }} /></span>
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
            <form className="login-form" onSubmit={submitAuthentication}>
              <div><strong>{authMode === 'register' ? '注册 bilibili-lite' : '登录 bilibili-lite'}</strong><p>{authMode === 'register' ? '创建账户后即可互动和投稿' : '互动、评论和投稿需要登录'}</p></div>
              <label><span>用户名</span><input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /></label>
              {authMode === 'register' && <label><span>显示名称</span><input autoComplete="nickname" maxLength={100} value={displayName} onChange={(event) => setDisplayName(event.target.value)} required /></label>}
              <label><span>密码</span><input type="password" minLength={authMode === 'register' ? 8 : undefined} maxLength={72} autoComplete={authMode === 'register' ? 'new-password' : 'current-password'} value={password} onChange={(event) => setPassword(event.target.value)} required /></label>
              {error && <p className="form-error" role="alert">{error}</p>}
              <button className="menu-primary" type="submit" disabled={pending}>{pending ? authMode === 'register' ? '注册中' : '登录中' : authMode === 'register' ? '注册并登录' : '登录'}</button>
              <button className="menu-quiet" type="button" disabled={pending} onClick={() => { setAuthMode((current) => current === 'login' ? 'register' : 'login'); setError(''); setPassword('') }}>{authMode === 'register' ? '已有账户，返回登录' : '没有账户，立即注册'}</button>
            </form>
          )}
        </div>
      )}
    </div>
  )
}

function sameUser(left: AuthUser, right: AuthUser) {
  return left.id === right.id &&
    left.username === right.username &&
    left.displayName === right.displayName &&
    left.avatarUrl === right.avatarUrl &&
    left.bio === right.bio &&
    left.coinBalance === right.coinBalance &&
    left.isAdmin === right.isAdmin &&
    left.experience === right.experience &&
    left.level === right.level
}
