import { ChevronDown, Heart, MessageCircle, Send, Trash2, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  authorizedFetch,
  authorizedJson,
  fetchJson,
  normalizeVideoComment,
  normalizeVideoCommentInteraction,
  normalizeVideoComments,
  toErrorMessage,
  toNumber,
} from '../api'
import { useAuth } from '../auth/useAuth'
import type { VideoComment } from '../types'
import { formatCount, formatDate } from '../utils/format'

type ReplyTarget = { rootId: number; parentId: number; userName: string }
type ReplyPage = { items: VideoComment[]; nextPageToken: string; loading: boolean; loaded: boolean }

export function CommentSection({ bvid, ownerId, onCountChange }: { bvid: string; ownerId?: number; onCountChange: (delta: number) => void }) {
  const { session, setSession } = useAuth()
  const [comments, setComments] = useState<VideoComment[]>([])
  const [replies, setReplies] = useState<Record<number, ReplyPage>>({})
  const [nextPageToken, setNextPageToken] = useState('')
  const [content, setContent] = useState('')
  const [replyTarget, setReplyTarget] = useState<ReplyTarget | null>(null)
  const [replyContent, setReplyContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    const query = new URLSearchParams({ page_size: '20' })
    const request = session
      ? authorizedJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/comments?${query}`, {}, session).then((result) => {
          setSession(result.session)
          return result.data
        })
      : fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/comments?${query}`)
    void request.then((payload) => {
      if (!active) return
      const page = normalizeVideoComments(payload)
      setComments(page.comments)
      setNextPageToken(page.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '评论加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
    // Reload when the viewer changes so the server can return their like state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bvid, session?.user.id])

  async function loadRootPage(pageToken: string) {
    const query = new URLSearchParams({ page_size: '20' })
    if (pageToken) query.set('page_token', pageToken)
    if (!session) return normalizeVideoComments(await fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/comments?${query}`))
    const result = await authorizedJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/comments?${query}`, {}, session)
    setSession(result.session)
    return normalizeVideoComments(result.data)
  }

  async function loadReplies(rootId: number, append = false) {
    const current = replies[rootId]
    if (current?.loading) return
    setReplies((pages) => ({ ...pages, [rootId]: { items: current?.items ?? [], nextPageToken: current?.nextPageToken ?? '', loading: true, loaded: current?.loaded ?? false } }))
    try {
      const query = new URLSearchParams({ page_size: '20' })
      if (append && current?.nextPageToken) query.set('page_token', current.nextPageToken)
      const url = `/api/v1/videos/${encodeURIComponent(bvid)}/comments/${rootId}/replies?${query}`
      let payload: unknown
      if (session) {
        const result = await authorizedJson<unknown>(url, {}, session)
        setSession(result.session)
        payload = result.data
      } else {
        payload = await fetchJson<unknown>(url)
      }
      const page = normalizeVideoComments(payload)
      setReplies((pages) => ({ ...pages, [rootId]: { items: append ? [...(pages[rootId]?.items ?? []), ...page.comments] : page.comments, nextPageToken: page.nextPageToken, loading: false, loaded: true } }))
    } catch (loadError) {
      setReplies((pages) => ({ ...pages, [rootId]: { ...(pages[rootId] ?? { items: [], nextPageToken: '', loaded: false }), loading: false } }))
      setError(toErrorMessage(loadError, '回复加载失败'))
    }
  }

  async function publish(contentValue: string, target: ReplyTarget | null) {
    if (!session) {
      setError('请先登录再发表评论')
      return
    }
    const value = contentValue.trim()
    if (!value) return
    setPending(true)
    setError('')
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/comments`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ content: value, parent_comment_id: target?.parentId || 0 }) },
        session,
      )
      setSession(result.session)
      const created = normalizeVideoComment(result.data)
      if (!target) {
        setComments((current) => [created, ...current])
        setContent('')
      } else {
        setComments((current) => current.map((item) => item.id === target.rootId ? { ...item, replyCount: toNumber(item.replyCount) + 1 } : item))
        setReplies((pages) => {
          const page = pages[target.rootId]
          if (!page?.loaded) return pages
          return { ...pages, [target.rootId]: { ...page, items: [...page.items, created] } }
        })
        setReplyContent('')
        setReplyTarget(null)
      }
      onCountChange(1)
    } catch (commentError) {
      setError(toErrorMessage(commentError, '评论发布失败'))
    } finally {
      setPending(false)
    }
  }

  async function deleteComment(comment: VideoComment) {
    if (!session) return
    setError('')
    try {
      const result = await authorizedFetch(`/api/v1/videos/${encodeURIComponent(bvid)}/comments/${comment.id}`, { method: 'DELETE' }, session)
      setSession(result.session)
      if (comment.parentId === 0) {
        setComments((current) => current.flatMap((item) => item.id !== comment.id ? [item] : toNumber(item.replyCount) > 0 ? [{ ...item, deleted: true, content: '' }] : []))
      } else {
        const rootId = comment.rootId
        setReplies((pages) => ({ ...pages, [rootId]: { ...pages[rootId], items: (pages[rootId]?.items ?? []).filter((item) => item.id !== comment.id) } }))
        setComments((current) => current.map((item) => item.id === rootId ? { ...item, replyCount: Math.max(0, toNumber(item.replyCount) - 1) } : item))
      }
      onCountChange(-1)
    } catch (deleteError) {
      setError(toErrorMessage(deleteError, '评论删除失败'))
    }
  }

  async function toggleLike(comment: VideoComment) {
    if (!session) {
      setError('请先登录再点赞')
      return
    }
    const desired = !comment.liked
    updateComment(comment, (item) => ({ ...item, liked: desired, likeCount: Math.max(0, toNumber(item.likeCount) + (desired ? 1 : -1)) }))
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/comments/${comment.id}/like`,
        { method: desired ? 'POST' : 'DELETE' }, session,
      )
      setSession(result.session)
      const interaction = normalizeVideoCommentInteraction(result.data)
      updateComment(comment, (item) => ({ ...item, liked: interaction.liked, likeCount: interaction.likeCount }))
    } catch (likeError) {
      updateComment(comment, () => comment)
      setError(toErrorMessage(likeError, '点赞失败'))
    }
  }

  function updateComment(comment: VideoComment, update: (item: VideoComment) => VideoComment) {
    if (comment.parentId === 0) setComments((current) => current.map((item) => item.id === comment.id ? update(item) : item))
    else setReplies((pages) => ({ ...pages, [comment.rootId]: { ...pages[comment.rootId], items: (pages[comment.rootId]?.items ?? []).map((item) => item.id === comment.id ? update(item) : item) } }))
  }

  async function loadMore() {
    try {
      const page = await loadRootPage(nextPageToken)
      setComments((current) => [...current, ...page.comments])
      setNextPageToken(page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页评论加载失败'))
    }
  }

  return (
    <section className="comment-section" aria-labelledby="comment-title">
      <header><h2 id="comment-title">评论</h2><span>{comments.length}{nextPageToken ? '+' : ''}</span></header>
      <form className="comment-composer" onSubmit={(event) => { event.preventDefault(); void publish(content, null) }}>
        <CommentAvatar name={session?.user.displayName || session?.user.username || '游客'} url={session?.user.avatarUrl} />
        <textarea maxLength={2000} rows={3} value={content} onChange={(event) => setContent(event.target.value)} placeholder={session ? '发一条友善的评论' : '登录后参与评论'} />
        <button className="primary-button" type="submit" disabled={pending || !content.trim()}><Send size={17} />{pending ? '发布中' : '发表评论'}</button>
      </form>
      {error && <p className="inline-error comment-error" role="status">{error}</p>}
      <div className="comment-list">
        {loading ? Array.from({ length: 3 }, (_, index) => <div className="comment-skeleton" key={index} />)
          : comments.length === 0 ? <p className="compact-empty">还没有评论，来留下第一条吧。</p>
            : comments.map((comment) => {
              const page = replies[comment.id]
              return <article className="comment-thread" key={comment.id}>
                <CommentRow comment={comment} ownerId={ownerId} onDelete={deleteComment} onLike={toggleLike} onReply={() => { setReplyTarget({ rootId: comment.id, parentId: comment.id, userName: comment.userName }); setReplyContent('') }} />
                {toNumber(comment.replyCount) > 0 && !page?.loaded && <button type="button" className="show-replies" disabled={page?.loading} onClick={() => void loadReplies(comment.id)}><MessageCircle size={15} />{page?.loading ? '加载中' : `查看 ${formatCount(comment.replyCount)} 条回复`}<ChevronDown size={15} /></button>}
                {page?.loaded && <div className="reply-list">{page.items.map((reply) => <CommentRow key={reply.id} comment={reply} ownerId={ownerId} compact onDelete={deleteComment} onLike={toggleLike} onReply={() => { setReplyTarget({ rootId: comment.id, parentId: reply.id, userName: reply.userName }); setReplyContent('') }} />)}{page.nextPageToken && <button type="button" className="show-replies" disabled={page.loading} onClick={() => void loadReplies(comment.id, true)}>查看更多回复</button>}</div>}
                {replyTarget?.rootId === comment.id && <form className="reply-composer" onSubmit={(event) => { event.preventDefault(); void publish(replyContent, replyTarget) }}><input autoFocus maxLength={2000} value={replyContent} onChange={(event) => setReplyContent(event.target.value)} placeholder={`回复 ${replyTarget.userName}`} /><button type="button" className="icon-button" title="取消回复" aria-label="取消回复" onClick={() => setReplyTarget(null)}><X size={16} /></button><button type="submit" className="send-button" title="发布回复" aria-label="发布回复" disabled={pending || !replyContent.trim()}><Send size={16} /></button></form>}
              </article>
            })}
      </div>
      {nextPageToken && <button className="load-comments" type="button" onClick={() => void loadMore()}>查看更多评论</button>}
    </section>
  )
}

function CommentRow({ comment, ownerId, compact = false, onDelete, onLike, onReply }: { comment: VideoComment; ownerId?: number; compact?: boolean; onDelete: (comment: VideoComment) => Promise<void>; onLike: (comment: VideoComment) => Promise<void>; onReply: () => void }) {
  const { session } = useAuth()
  const canDelete = session && (session.user.id === comment.userId || session.user.id === ownerId)
  return <div className={`comment-row ${compact ? 'compact' : ''}`}>
    <Link to={`/space/${comment.userId}`} aria-label={`${comment.userName} 的主页`}><CommentAvatar name={comment.userName} url={comment.userAvatarUrl} /></Link>
    <div className="comment-content">
      <Link className="comment-author" to={`/space/${comment.userId}`}>{comment.userName}</Link>
      <p className={comment.deleted ? 'deleted-comment' : ''}>{comment.deleted ? '该评论已删除' : <>{comment.replyToUserName && <span className="reply-prefix">回复 {comment.replyToUserName}：</span>}{comment.content}</>}</p>
      {!comment.deleted && <div className="comment-meta"><time>{formatDate(comment.createdAt)}</time><button type="button" className={comment.liked ? 'active' : ''} aria-label={comment.liked ? '取消点赞评论' : '点赞评论'} title={comment.liked ? '取消点赞评论' : '点赞评论'} onClick={() => void onLike(comment)}><Heart size={15} fill={comment.liked ? 'currentColor' : 'none'} />{toNumber(comment.likeCount) > 0 && formatCount(comment.likeCount)}</button><button type="button" onClick={onReply}><MessageCircle size={15} />回复</button>{canDelete && <button type="button" title="删除评论" onClick={() => void onDelete(comment)}><Trash2 size={15} />删除</button>}</div>}
    </div>
  </div>
}

function CommentAvatar({ name, url }: { name: string; url?: string }) {
  return <span className="comment-avatar">{url ? <img src={url} alt="" /> : (name.slice(0, 1) || '用')}</span>
}
