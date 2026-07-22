import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  clearAuthSession,
  ensureFreshAuthSession,
  persistAuthSession,
  restoreAuthSession,
} from '../api'
import type { AuthSession } from '../types'
import { AuthContext } from './auth-context'

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [session, setSessionState] = useState<AuthSession | null>(null)
  const [restoring, setRestoring] = useState(true)

  const setSession = useCallback((next: AuthSession | null) => {
    setSessionState(next)
    if (next) persistAuthSession(next)
    else clearAuthSession()
  }, [])

  useEffect(() => {
    let active = true
    void restoreAuthSession().then((restored) => {
      if (active) {
        setSessionState(restored)
        setRestoring(false)
      }
    })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    if (!session) return
    const refreshAt = new Date(session.expiresAt).getTime() - Date.now() - 60_000
    const timer = window.setTimeout(() => {
      void ensureFreshAuthSession(session)
        .then(setSession)
        .catch(() => setSession(null))
    }, Math.max(refreshAt, 0))
    return () => window.clearTimeout(timer)
  }, [session, setSession])

  const value = useMemo(() => ({ session, restoring, setSession }), [restoring, session, setSession])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
