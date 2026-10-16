import { authorizedJson, fetchJson } from './api'
import type { AuthSession } from './types'

// Protobuf JSON encodes int64 money and inventory values as strings.

export type TicketType = {
  id: string
  name: string
  priceCents: string
  available: string
}

export type MallProduct = {
  id: string
  name: string
  description: string
  year: number
  saleStartsAt: string
  ticketTypes: TicketType[]
}

export type MallOrder = {
  id: string
  productId: string
  ticketTypeId: string
  state: 'pending' | 'paid' | 'closed' | 'refunding' | 'refunded'
  paymentState: string
  expiresAt: string
  priceCents: string
  ticketTypeName: string
}

export type Purchase = { order: MallOrder | null; purchaseBlocked: boolean }

export type Attempt = { requestId: string; ticketTypeId: string }

export const money = (cents: string | number) =>
  (Number(cents) / 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })

export const mallProducts = (query: string, signal?: AbortSignal) =>
  fetchJson<{ products: MallProduct[] }>(
    `/api/mall/v1/products?query=${encodeURIComponent(query)}`,
    { signal },
  )

export async function mallAction<T>(
  path: string,
  session: AuthSession,
  setSession: (s: AuthSession) => void,
  body?: object,
) {
  const result = await authorizedJson<T>(
    `/api/mall/v1/${path}`,
    body === undefined
      ? {}
      : {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        },
    session,
  )
  if (result.session.accessToken !== session.accessToken) setSession(result.session)
  return result.data
}

export function readAttempt(key: string): Attempt | null {
  try {
    const a = JSON.parse(sessionStorage.getItem(key) || 'null')
    return typeof a?.requestId === 'string' && typeof a?.ticketTypeId === 'string' ? a : null
  } catch {
    return null
  }
}

export function saveAttempt(key: string, value: Attempt | null) {
  try {
    if (value) sessionStorage.setItem(key, JSON.stringify(value))
    else sessionStorage.removeItem(key)
  } catch {
    /* The in-memory attempt still protects retries when storage is unavailable. */
  }
}
