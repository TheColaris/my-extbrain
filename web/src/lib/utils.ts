import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

const SAFE_URL_PROTOCOLS = new Set(['http:', 'https:', 'mailto:', 'tel:'])

/** Sanitize a URL to prevent javascript: protocol injection. Returns '#' for unsafe values. */
export function safeHref(url: string | undefined): string {
  if (!url) return '#'
  if (url.startsWith('/') || url.startsWith('#')) return url
  try {
    const parsed = new URL(url, 'https://placeholder.invalid')
    return SAFE_URL_PROTOCOLS.has(parsed.protocol) ? url : '#'
  } catch {
    return '#'
  }
}

/** Sanitize a CSS value by stripping characters that could break out of a CSS declaration. */
export function sanitizeCssValue(value: string): string {
  return value.replace(/[{}<>;"'\\]/g, '')
}

/** 相对时间（回收站/通知记录行内展示）：3 小时前 / 昨天 22:40 / 25 天前 */
export function relTime(iso: string): string {
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return iso
  const diff = Date.now() - t
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  const days = Math.floor(diff / 86_400_000)
  const d = new Date(iso)
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (days === 1) return `昨天 ${hm}`
  if (days < 7) return `${days} 天前`
  return `${d.getMonth() + 1}-${pad(d.getDate())} ${hm}`
}

function pad(n: number): string {
  return String(n).padStart(2, '0')
}
