import { useEffect, useRef, useState } from 'react'
import { useOutletContext } from 'react-router-dom'
import {
  ArrowRight,
  Check,
  ChevronRight,
  Clock3,
  Image,
  Search,
  ShieldCheck,
  Ticket,
  X,
} from 'lucide-react'
import { APIError } from '../api'
import { useAuth } from '../auth/useAuth'
import { mallAction, mallProducts, money, readAttempt, saveAttempt } from '../mall'
import type { Attempt, MallOrder, MallProduct, Purchase } from '../mall'

export function MallPage() {
  const [query, setQuery] = useState('')
  const [search, setSearch] = useState('')
  const [reload, setReload] = useState(0)
  const [products, setProducts] = useState<MallProduct[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selected, setSelected] = useState<MallProduct | null>(null)
  const { requestLogin } = useOutletContext<{ requestLogin: () => void }>()
  const { session } = useAuth()
  useEffect(() => {
    const controller = new AbortController()
    mallProducts(search, controller.signal)
      .then((data) => {
        setProducts(data.products)
        setError('')
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(e instanceof Error ? e.message : '商品加载失败')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [search, reload])
  return (
    <main className="mall-page">
      <div className="mall-heading">
        <div className="mall-brand">
          <Ticket size={26} />
          <h1>会员购</h1>
          <span>和热爱见面</span>
        </div>
        <span className="mall-demo">演示场次 · 不产生真实消费</span>
      </div>
      <form
        className="mall-search"
        role="search"
        onSubmit={(e) => {
          e.preventDefault()
          setLoading(true)
          setSearch(query.trim())
          setReload((n) => n + 1)
        }}
      >
        <Search size={21} aria-hidden="true" />
        <input
          aria-label="搜索门票"
          placeholder="搜索 BILIBILI WORLD、年份或门票"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          maxLength={100}
        />
        <button type="submit">搜索</button>
      </form>
      <div className="mall-section-heading">
        <div>
          <span className="mall-category">展会门票</span>
          <span className="mall-count">{loading ? '加载中' : `${products.length} 个场次`}</span>
        </div>
        <span className="mall-year-range">BILIBILI WORLD · 2023—2026</span>
      </div>
      {error ? (
        <div className="mall-empty" role="alert">
          <h2>暂时无法加载门票</h2>
          <p>请稍后重试，已创建的订单不会受影响。</p>
          <button
            onClick={() => {
              setLoading(true)
              setReload((n) => n + 1)
            }}
          >
            重新加载
          </button>
        </div>
      ) : loading ? (
        <div className="mall-grid" aria-label="正在加载门票" aria-busy="true">
          {[0, 1, 2, 3].map((n) => (
            <div className="mall-skeleton" key={n} />
          ))}
        </div>
      ) : products.length === 0 ? (
        <div className="mall-empty">
          <Search size={32} />
          <h2>{search ? `没有找到“${search}”` : '还没有上架的门票'}</h2>
          <p>{search ? '试试搜索 BW、BILIBILI 或年份。' : '新的场次上架后，会在这里与你见面。'}</p>
        </div>
      ) : (
        <div className="mall-grid">
          {products.map((p) => (
            <button
              className="mall-product"
              key={p.id}
              onClick={() => setSelected(p)}
              aria-label={`查看 ${p.name}`}
            >
              <Poster product={p} />
              <div className="mall-product-info">
                <div className="mall-product-tags">
                  <span>展会</span>
                  <span>电子票</span>
                </div>
                <h2>{p.name}</h2>
                <p>{p.description || '选择你的票种，一起奔赴热爱。'}</p>
                <div className="mall-product-note">
                  <ShieldCheck size={15} />
                  每账号限购一张
                </div>
                <div className="mall-product-bottom">
                  <span className="mall-price">
                    <small>¥</small>
                    {money(Math.min(...p.ticketTypes.map((t) => Number(t.priceCents))))}
                    <em>起</em>
                  </span>
                  <span className="mall-view">
                    选票
                    <ChevronRight size={17} />
                  </span>
                </div>
              </div>
            </button>
          ))}
        </div>
      )}
      <p className="mall-footnote">以上为示例门票，价格与开售安排仅用于体验购票流程。</p>
      {selected && (
        <PurchaseOverlay
          key={`${selected.id}:${session?.user.id || 'guest'}`}
          product={selected}
          onClose={() => setSelected(null)}
          onLogin={() => {
            setSelected(null)
            requestLogin()
          }}
        />
      )}
    </main>
  )
}

function Poster({ product }: { product: MallProduct }) {
  return (
    <div
      className={`mall-poster mall-poster-${product.year}`}
      role="img"
      aria-label={`${product.year || ''} BW 海报占位`}
    >
      <span className="mall-poster-wordmark">
        BILIBILI
        <br />
        WORLD
      </span>
      <strong>{String(product.year || 'BW').slice(-2)}</strong>
      <span className="mall-poster-caption">
        <Image size={13} />
        海报待更新
      </span>
    </div>
  )
}

function PurchaseOverlay({
  product,
  onClose,
  onLogin,
}: {
  product: MallProduct
  onClose: () => void
  onLogin: () => void
}) {
  const { session, setSession } = useAuth()
  const dialog = useRef<HTMLDialogElement>(null)
  const lock = useRef(false)
  // A completed mutation invalidates older polls so stale responses cannot overwrite its result.
  const version = useRef(0)
  // Keep an uncertain purchase attempt across closing/reopening this event's overlay.
  const key = `mall.attempt:${session?.user.id || 'guest'}:${product.id}`
  const [attempt, setAttempt] = useState<Attempt | null>(() => readAttempt(key))
  const [typeId, setTypeId] = useState(
    () => readAttempt(key)?.ticketTypeId || product.ticketTypes[0]?.id || '',
  )
  const [order, setOrder] = useState<MallOrder | null>(null)
  const [blocked, setBlocked] = useState(false)
  const [checking, setChecking] = useState(Boolean(session))
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [cooldownUntil, setCooldownUntil] = useState(0)
  const [now, setNow] = useState(() => Date.now())
  const [refundConfirm, setRefundConfirm] = useState(false)
  const [checkRetry, setCheckRetry] = useState(0)
  const [checkFailed, setCheckFailed] = useState(false)
  const ticketType = product.ticketTypes.find(
    (t) => t.id === (order && order.state !== 'closed' ? order.ticketTypeId : typeId),
  )
  const remaining = order ? Math.max(0, Math.ceil((Date.parse(order.expiresAt) - now) / 1000)) : 0
  const cooldown = Math.max(0, Math.ceil((cooldownUntil - now) / 1000))
  const opensIn = Math.max(0, Math.ceil((Date.parse(product.saleStartsAt) - now) / 1000))
  const orderId = order?.id
  const orderState = order?.state
  const paymentState = order?.paymentState
  const pending = order?.state === 'pending'
  const paid = order?.state === 'paid'
  const refunded = order?.state === 'refunded'
  const refunding = order?.state === 'refunding' || order?.paymentState === 'refund_pending'
  const clearAttempt = () => {
    saveAttempt(key, null)
    setAttempt(null)
  }

  useEffect(() => {
    const element = dialog.current
    element?.showModal()
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      element?.close()
      document.body.style.overflow = overflow
    }
  }, [])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 250)
    return () => window.clearInterval(timer)
  }, [])
  useEffect(() => {
    if (!session) return
    let active = true
    mallAction<Purchase>(`products/${product.id}/purchase`, session, setSession)
      .then((p) => {
        if (!active) return
        setOrder(p.order)
        setBlocked(p.purchaseBlocked)
        setCheckFailed(false)
        setMessage('')
        if (p.order) {
          saveAttempt(key, null)
          setAttempt(null)
        }
      })
      .catch(() => {
        if (active) {
          setMessage('未能确认当前购票状态，请重试。')
          setCheckFailed(true)
        }
      })
      .finally(() => {
        if (active) setChecking(false)
      })
    return () => {
      active = false
    }
  }, [session, product.id, setSession, key, checkRetry])
  useEffect(() => {
    if (
      !session ||
      !orderId ||
      (!['pending', 'refunding'].includes(orderState || '') && paymentState !== 'refund_pending')
    )
      return
    let active = true
    const timer = window.setInterval(() => {
      if (lock.current) return
      const requestedVersion = version.current
      mallAction<MallOrder>(`orders/${orderId}`, session, setSession)
        .then((o) => {
          if (active && version.current === requestedVersion && !lock.current) setOrder(o)
        })
        .catch(() => {
          if (active) setMessage('订单状态更新暂时中断，请稍后重新确认。')
        })
    }, 3000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [orderId, orderState, paymentState, session, setSession])

  async function act(action: 'buy' | 'pay' | 'refund') {
    if (!session) {
      onLogin()
      return
    }
    if (lock.current) return
    lock.current = true
    version.current += 1
    setBusy(true)
    setMessage('')
    // Network failure does not prove rejection. Reuse this key until the outcome is known.
    let nextAttempt = attempt
    if (action === 'buy' && !nextAttempt) {
      nextAttempt = { requestId: crypto.randomUUID(), ticketTypeId: typeId }
      saveAttempt(key, nextAttempt)
      setAttempt(nextAttempt)
    }
    try {
      const path = action === 'buy' ? 'orders' : `orders/${order!.id}/${action}`
      const body = action === 'buy' ? { productId: product.id, ...nextAttempt } : {}
      const result = await mallAction<MallOrder>(path, session, setSession, body)
      setOrder(result)
      clearAttempt()
      setRefundConfirm(false)
      if (result.state === 'paid' || result.state === 'refunded') setBlocked(true)
    } catch (error) {
      const code = error instanceof APIError ? String(error.metadata.business_code || '') : ''
      const retry = error instanceof APIError ? Number(error.metadata.retry_after_ms || 0) : 0
      if (retry > 0) setCooldownUntil(Date.now() + retry)
      const messages: Record<string, string> = {
        '429': '购票人数较多，请稍后再试。',
        '412': '请求过于频繁，请稍后再试。',
        '900001': '操作太快，请稍等一秒。',
        '3': '抢票冷却中，倒计时结束后可再次尝试。',
        '100009': '该票种暂时无票，可稍后再试或选择其他票种。',
        '100019': '该账号已有订单或购票记录，请关闭弹层后重新查看。',
        '219': '余票刚刚售罄，请稍后再试。',
        '221': '当前购票人数较多，请等待后重试。',
      }
      setMessage(
        messages[code] ||
          (error instanceof APIError && error.reason === 'MALL_NOT_ON_SALE'
            ? '该场次暂未开售，请在开售后重试。'
            : error instanceof APIError && error.reason === 'MALL_ADMISSION_UNAVAILABLE'
              ? '抢票服务暂不可用，请稍后重试。'
              : error instanceof APIError && error.status === 401
                ? '登录已失效，请重新登录。'
                : '暂未确认操作结果，请重试确认。重复操作不会重复扣款或占票。'),
      )
      // Only definitive admission rejections can discard the idempotency key.
      if (
        action === 'buy' &&
        (code || (error instanceof APIError && error.status >= 400 && error.status < 500))
      )
        clearAttempt()
    } finally {
      lock.current = false
      setBusy(false)
    }
  }

  return (
    <dialog
      ref={dialog}
      className="mall-dialog"
      aria-labelledby="mall-dialog-title"
      onCancel={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="mall-dialog-content">
        <div className="mall-dialog-top">
          <span>
            <Ticket size={18} />
            门票预订
          </span>
          <button className="mall-close" aria-label="关闭购票窗口" onClick={onClose}>
            <X size={22} />
          </button>
        </div>
        <div className="mall-dialog-summary">
          <Poster product={product} />
          <div>
            <span className="mall-demo">演示场次</span>
            <h2 id="mall-dialog-title">{product.name}</h2>
            <p>{product.description}</p>
            <span className="mall-dialog-meta">电子票 · 每账号限购一张</span>
          </div>
        </div>
        {!order || order.state === 'closed' ? (
          <>
            <fieldset
              className="mall-ticket-options"
              disabled={busy || Boolean(attempt) || blocked || checking || checkFailed}
            >
              <legend>选择票种</legend>
              {product.ticketTypes.map((t) => (
                <label key={t.id} className={typeId === t.id ? 'is-selected' : ''}>
                  <input
                    type="radio"
                    name="ticket-type"
                    value={t.id}
                    checked={typeId === t.id}
                    onChange={() => setTypeId(t.id)}
                  />
                  <span>{t.name}</span>
                  <strong>¥{money(t.priceCents)}</strong>
                  {typeId === t.id && <Check size={16} />}
                </label>
              ))}
            </fieldset>
            <div className="mall-purchase-rules">
              <h3>购票须知</h3>
              <p>
                订单创建后为你保留门票 10
                分钟，超时未支付将自动取消。取消后可重新抢购；退票后，本账号不可再次购买该场次任意票种。
              </p>
            </div>
            {order?.state === 'closed' && (
              <p className="mall-status-note">
                订单已超时关闭，可以重新选票。
                {order.paymentState === 'refund_pending' && '原支付正在自动退回。'}
              </p>
            )}
          </>
        ) : (
          <div className={`mall-order-state ${paid ? 'is-paid' : ''}`}>
            <div className="mall-order-state-title">
              {paid ? <Check size={24} /> : <Clock3 size={24} />}
              <h3>
                {paid
                  ? '购票成功'
                  : refunded
                    ? '已退票'
                    : refunding
                      ? '退款处理中'
                      : remaining
                        ? '门票已为你锁定'
                        : '订单到期，正在确认关闭'}
              </h3>
            </div>
            <p>
              {paid
                ? '你的门票已确认，期待与你在 BW 相见。'
                : refunded
                  ? '款项已模拟退回，本账号不可再次购买该场次。'
                  : refunding
                    ? '退款结果会自动更新，请稍候。'
                    : '请在保留时间内完成支付，超时后库存将自动释放。'}
            </p>
            <dl>
              <div>
                <dt>票种</dt>
                <dd>{order.ticketTypeName} × 1</dd>
              </div>
              <div>
                <dt>订单金额</dt>
                <dd>¥{money(order.priceCents)}</dd>
              </div>
              {pending && (
                <div>
                  <dt>支付剩余时间</dt>
                  <dd className="mall-countdown">
                    {Math.floor(remaining / 60)
                      .toString()
                      .padStart(2, '0')}
                    :{(remaining % 60).toString().padStart(2, '0')}
                  </dd>
                </div>
              )}
            </dl>
            <small className="mall-order-number">订单号 {order.id}</small>
          </div>
        )}
        {message && (
          <p className="mall-feedback" role="status">
            {message}
          </p>
        )}
        {checkFailed && (
          <button
            className="mall-text-button"
            onClick={() => {
              setChecking(true)
              setCheckRetry((n) => n + 1)
            }}
          >
            重新确认购票状态
          </button>
        )}
        {refundConfirm && (
          <div className="mall-refund-confirm">
            <strong>确认退票？</strong>
            <p>退票后无法再次购买本场次，包括其他票种。</p>
            <button onClick={() => setRefundConfirm(false)} disabled={busy}>
              保留门票
            </button>
            <button onClick={() => void act('refund')} disabled={busy}>
              确认退票
            </button>
          </div>
        )}
        <div className="mall-dialog-footer">
          <div>
            <span>{paid || refunded ? '订单金额' : '合计'}</span>
            <strong>
              ¥
              {money(
                order && order.state !== 'closed'
                  ? order.priceCents
                  : ticketType?.priceCents || '0',
              )}
            </strong>
            <small>1 张</small>
          </div>
          {paid ? (
            <button
              className="mall-secondary"
              onClick={() => setRefundConfirm(true)}
              disabled={busy || refundConfirm}
            >
              申请退票
            </button>
          ) : refunding ? (
            <button className="mall-primary" disabled>
              退款处理中
            </button>
          ) : refunded || (blocked && !pending) ? (
            <button className="mall-secondary" onClick={onClose}>
              完成
            </button>
          ) : (
            <button
              className="mall-primary"
              disabled={
                busy ||
                checking ||
                checkFailed ||
                cooldown > 0 ||
                opensIn > 0 ||
                (pending && remaining === 0) ||
                !ticketType
              }
              onClick={() => void act(pending ? 'pay' : 'buy')}
            >
              {busy
                ? '正在确认…'
                : checking
                  ? '确认购票状态…'
                  : cooldown > 0
                    ? `${cooldown} 秒后重试`
                    : !session
                      ? '登录后购票'
                      : opensIn > 0
                        ? '暂未开售'
                        : pending
                          ? '模拟支付'
                          : attempt
                            ? '重试确认订单'
                            : '立即抢购'}
              {!busy && !cooldown && <ArrowRight size={17} />}
            </button>
          )}
        </div>
        <p className="mall-payment-note">模拟支付，不会实际扣款</p>
      </div>
    </dialog>
  )
}
