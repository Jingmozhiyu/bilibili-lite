import { CheckCircle2, FileVideo, ImagePlus, RotateCcw, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  asRecord,
  authorizedFetch,
  ensureFreshAuthSession,
  parseJSON,
  readString,
  toErrorMessage,
} from '../api'
import { useAuth } from '../auth/useAuth'
import type { UploadResult } from '../types'
import { formatFileSize, splitTags } from '../utils/format'

type UploadPhase = 'idle' | 'uploading' | 'processing' | 'ready' | 'publishing' | 'success' | 'error'

export function UploadPanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { session, setSession } = useAuth()
  const navigate = useNavigate()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [videoFile, setVideoFile] = useState<File | null>(null)
  const [coverFile, setCoverFile] = useState<File | null>(null)
  const [phase, setPhase] = useState<UploadPhase>('idle')
  const [progress, setProgress] = useState(0)
  const [message, setMessage] = useState('')
  const [result, setResult] = useState<UploadResult | null>(null)
  const requestRef = useRef<XMLHttpRequest | null>(null)

  useEffect(() => () => requestRef.current?.abort(), [])

  async function startUpload(file: File) {
    if (!session) return
    requestRef.current?.abort()
    setVideoFile(file)
    setPhase('uploading')
    setProgress(0)
    setMessage('')
    setResult(null)
    try {
      const activeSession = await ensureFreshAuthSession(session)
      setSession(activeSession)
      const form = new FormData()
      if (coverFile) form.append('cover', coverFile)
      form.append('file', file)
      const uploadResult = await new Promise<UploadResult>((resolve, reject) => {
        const request = new XMLHttpRequest()
        requestRef.current = request
        request.open('POST', '/api/v1/videos/upload')
        request.setRequestHeader('Authorization', `Bearer ${activeSession.accessToken}`)
        request.upload.addEventListener('progress', (event) => {
          if (!event.lengthComputable) return
          const nextProgress = Math.round((event.loaded / event.total) * 100)
          setProgress(nextProgress)
          if (nextProgress >= 100) setPhase('processing')
        })
        request.addEventListener('load', () => {
          requestRef.current = null
          const payload = asRecord(parseJSON(request.responseText))
          if (request.status < 200 || request.status >= 300) {
            reject(new Error(readString(payload, 'message') || `上传失败（${request.status}）`))
            return
          }
          resolve({
            bvid: readString(payload, 'bvid'), status: readString(payload, 'status'),
            manifestUrl: readString(payload, 'manifestUrl', 'manifest_url'),
            coverUrl: readString(payload, 'coverUrl', 'cover_url'),
            videoUrl: readString(payload, 'videoUrl', 'video_url'),
          })
        })
        request.addEventListener('error', () => reject(new Error('网络连接中断，上传已停止')))
        request.addEventListener('abort', () => reject(new Error('上传已取消')))
        request.send(form)
      })
      setResult(uploadResult)
      setPhase('ready')
      setMessage('视频处理完成，填写信息后即可发布')
    } catch (uploadError) {
      setPhase('error')
      setMessage(toErrorMessage(uploadError, '上传失败'))
    }
  }

  async function publish(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!session || !result || !title.trim()) return
    setPhase('publishing')
    setMessage('')
    try {
      const { session: nextSession } = await authorizedFetch(
        `/api/v1/videos/${encodeURIComponent(result.bvid)}/publish`,
        {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ title, description, tags: splitTags(tags) }),
        },
        session,
      )
      setSession(nextSession)
      setPhase('success')
      setMessage('发布成功')
      window.setTimeout(() => {
        onClose()
        navigate(`/video/${result.bvid}`)
      }, 500)
    } catch (publishError) {
      setPhase('error')
      setMessage(toErrorMessage(publishError, '发布失败'))
    }
  }

  if (!open) return null
  const uploadLocked = phase === 'uploading' || phase === 'processing' || phase === 'ready' || phase === 'publishing' || phase === 'success'

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <section className="upload-dialog" role="dialog" aria-modal="true" aria-labelledby="upload-dialog-title">
        <header className="dialog-heading">
          <div><strong id="upload-dialog-title">投稿视频</strong><span>选择视频后自动上传，期间可以继续填写信息</span></div>
          <button type="button" className="icon-button" aria-label="关闭投稿" title="关闭" onClick={onClose}><X size={20} /></button>
        </header>
        <form className="upload-form" onSubmit={publish}>
          <div className="upload-pickers">
            <label className={uploadLocked ? 'file-picker disabled' : 'file-picker'}>
              <ImagePlus size={22} /><span>{coverFile ? coverFile.name : '自定义封面（可选）'}</span>
              <small>{coverFile ? formatFileSize(coverFile.size) : '先选封面；留空则自动截帧'}</small>
              <input type="file" disabled={uploadLocked} accept="image/jpeg,image/png,image/webp,.jpg,.jpeg,.png,.webp" onChange={(event) => setCoverFile(event.target.files?.[0] ?? null)} />
            </label>
            <label className={uploadLocked ? 'file-picker disabled' : 'file-picker primary'}>
              <FileVideo size={22} /><span>{videoFile ? videoFile.name : '选择 MP4 视频'}</span>
              <small>{videoFile ? formatFileSize(videoFile.size) : '选择后立即开始上传'}</small>
              <input type="file" disabled={uploadLocked} accept="video/mp4,.mp4" onChange={(event) => { const file = event.target.files?.[0]; if (file) void startUpload(file) }} />
            </label>
          </div>

          {phase !== 'idle' && (
            <div className={`upload-progress-panel ${phase}`} aria-live="polite">
              <div><span>{phase === 'uploading' ? '正在上传' : phase === 'processing' ? '正在转码和切片' : phase === 'ready' ? '等待发布' : phase === 'publishing' ? '正在发布' : phase === 'success' ? '发布完成' : '处理失败'}</span><strong>{phase === 'uploading' ? `${progress}%` : result?.bvid || ''}</strong></div>
              <div className="progress-track"><span style={{ width: phase === 'processing' ? '72%' : phase === 'ready' || phase === 'success' ? '100%' : `${progress}%` }} /></div>
              {message && <p>{message}</p>}
              {phase === 'error' && videoFile && <button type="button" className="retry-button" onClick={() => void startUpload(videoFile)}><RotateCcw size={16} />重新上传</button>}
            </div>
          )}

          <div className="metadata-fields">
            <label><span>标题</span><input maxLength={200} required value={title} onChange={(event) => setTitle(event.target.value)} placeholder="填写清晰、准确的视频标题" /></label>
            <label><span>简介</span><textarea maxLength={10000} rows={4} value={description} onChange={(event) => setDescription(event.target.value)} placeholder="介绍一下视频内容" /></label>
            <label><span>标签</span><input value={tags} onChange={(event) => setTags(event.target.value)} placeholder="用逗号分隔，最多 12 个" /></label>
          </div>
          <footer className="dialog-actions">
            <span>{result ? `${result.bvid} 已占用并完成处理` : '视频进入数据库后会立即获得 BV 号'}</span>
            <button className="primary-button" type="submit" disabled={!result || !title.trim() || phase === 'publishing' || phase === 'success'}>
              {phase === 'success' ? <CheckCircle2 size={18} /> : null}{phase === 'publishing' ? '发布中' : phase === 'success' ? '已发布' : '发布视频'}
            </button>
          </footer>
        </form>
      </section>
    </div>
  )
}
