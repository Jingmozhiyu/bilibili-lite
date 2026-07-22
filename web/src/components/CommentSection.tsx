import { Send, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import {
  authorizedFetch,
  authorizedJson,
  fetchJson,
  normalizeVideoComment,
  normalizeVideoComments,
  toErrorMessage,
} from '../api'
import { useAuth } from '../auth/useAuth'
import type { VideoComment } from '../types'
import { formatDate } from '../utils/format'

export function CommentSection({ bvid, ownerId, onCountChange }: { bvid: string; ownerId?: number; onCountChange: (delta: number) => void }) {
  const { session, setSession } = useAuth()
  const [comments, setComments] = useState<VideoComment[]>([])
  const [nextPageToken, setNextPageToken] = useState('')
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  const loadComments = useCallback(async (pageToken: string) => {
    const query = new URLSearchParams({ page_size: '20' })
    if (pageToken) query.set('page_token', pageToken)
    return normalizeVideoComments(await fetchJson<unknown>(`/api/v1/videos/${encodeURIComponent(bvid)}/comments?${query}`))
  }, [bvid])

  useEffect(() => {
    let active = true
    void loadComments('').then((page) => {
      if (!active) return
      setComments(page.comments)
      setNextPageToken(page.nextPageToken)
    }).catch((loadError) => {
      if (active) setError(toErrorMessage(loadError, '评论加载失败'))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [loadComments])

  async function createComment(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!session) {
      setError('请先登录再发表评论')
      return
    }
    if (!content.trim()) return
    setPending(true)
    setError('')
    try {
      const result = await authorizedJson<unknown>(
        `/api/v1/videos/${encodeURIComponent(bvid)}/comments`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ content }) },
        session,
      )
      setSession(result.session)
      setComments((current) => [normalizeVideoComment(result.data), ...current])
      setContent('')
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
      const result = await authorizedFetch(
        `/api/v1/videos/${encodeURIComponent(bvid)}/comments/${comment.id}`,
        { method: 'DELETE' }, session,
      )
      setSession(result.session)
      setComments((current) => current.filter((item) => item.id !== comment.id))
      onCountChange(-1)
    } catch (deleteError) {
      setError(toErrorMessage(deleteError, '评论删除失败'))
    }
  }

  async function loadMore() {
    try {
      const page = await loadComments(nextPageToken)
      setComments((current) => [...current, ...page.comments])
      setNextPageToken(page.nextPageToken)
    } catch (loadError) {
      setError(toErrorMessage(loadError, '下一页评论加载失败'))
    }
  }

  return (
    <section className="comment-section" aria-labelledby="comment-title">
      <header><h2 id="comment-title">评论</h2><span>{comments.length}{nextPageToken ? '+' : ''}</span></header>
      <form className="comment-composer" onSubmit={createComment}>
        <span className="comment-avatar">{session ? (session.user.displayName || session.user.username).slice(0, 1) : '游'}</span>
        <textarea maxLength={2000} rows={3} value={content} onChange={(event) => setContent(event.target.value)} placeholder={session ? '发一条友善的评论' : '登录后参与评论'} />
        <button className="primary-button" type="submit" disabled={pending || !content.trim()}><Send size={17} />{pending ? '发布中' : '发表评论'}</button>
      </form>
      {error && <p className="inline-error" role="status">{error}</p>}
      <div className="comment-list">
        {loading ? Array.from({ length: 3 }, (_, index) => <div className="comment-skeleton" key={index} />)
          : comments.length === 0 ? <p className="compact-empty">还没有评论，来留下第一条吧。</p>
            : comments.map((comment) => {
              const canDelete = session && (session.user.id === comment.userId || session.user.id === ownerId)
              return <article className="comment-item" key={comment.id}>
                <span className="comment-avatar">{comment.userName.slice(0, 1) || '用'}</span>
                <div><strong>{comment.userName}</strong><p>{comment.content}</p><time>{formatDate(comment.createdAt)}</time></div>
                {canDelete && <button type="button" className="icon-button" title="删除评论" aria-label="删除评论" onClick={() => void deleteComment(comment)}><Trash2 size={16} /></button>}
              </article>
            })}
      </div>
      {nextPageToken && <button className="load-comments" type="button" onClick={() => void loadMore()}>查看更多评论</button>}
    </section>
  )
}
