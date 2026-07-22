import { createContext } from 'react'
import type { AuthSession } from '../types'

export type AuthContextValue = {
  session: AuthSession | null
  restoring: boolean
  setSession: (session: AuthSession | null) => void
}

export const AuthContext = createContext<AuthContextValue | null>(null)
