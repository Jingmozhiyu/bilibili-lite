import type { MetricValue } from '../types'
import { toNumber } from '../api'

export function formatCount(value: MetricValue | undefined) {
  const count = toNumber(value)
  if (count >= 100_000_000) return `${(count / 100_000_000).toFixed(1)}亿`
  if (count >= 10_000) return `${(count / 10_000).toFixed(1)}万`
  return new Intl.NumberFormat('zh-CN').format(count)
}

export function formatBitrate(value: number) {
  if (!value) return '-'
  return `${(value / 1_000_000).toFixed(1)} Mbps`
}

export function formatDuration(value: MetricValue | undefined) {
  const seconds = Math.max(0, Math.floor(toNumber(value)))
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const remainder = seconds % 60
  if (hours > 0) return `${hours}:${minutes.toString().padStart(2, '0')}:${remainder.toString().padStart(2, '0')}`
  return `${minutes}:${remainder.toString().padStart(2, '0')}`
}

export function formatDate(value?: string) {
  if (!value) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  }).format(new Date(value))
}

export function formatShortDate(value?: string) {
  if (!value) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit' }).format(new Date(value))
}

export function formatFileSize(bytes: number) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(2)} GB`
  return `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

export function splitTags(value: string) {
  return value.split(/[,，\n]/).map((tag) => tag.trim()).filter(Boolean)
}
