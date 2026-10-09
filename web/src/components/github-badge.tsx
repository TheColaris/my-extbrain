import { useEffect, useState } from 'react'

/* GitHub 开源徽章（landing 顶栏 + 页脚，2026-10-09 用户令；取数口径=前端直连 GitHub API）。
   稳定性处理：5s 超时 + 模块级 promise 复用（顶栏/页脚两实例只发一次请求）+ sessionStorage 缓存 1 小时
   （未认证限流 60/h/IP，缓存后正常浏览几乎不消耗）；任何失败静默降级 = 只显示 logo/链接、不显示数字。 */

const REPO = 'TheColaris/my-extbrain'
const REPO_URL = `https://github.com/${REPO}`
const CACHE_KEY = 'extbrain:gh-stars'
const CACHE_TTL = 60 * 60 * 1000

let starsPromise: Promise<number | null> | null = null

function loadStars(): Promise<number | null> {
  if (starsPromise) return starsPromise
  try {
    const raw = sessionStorage.getItem(CACHE_KEY)
    if (raw) {
      const { n, t } = JSON.parse(raw) as { n: number; t: number }
      if (Date.now() - t < CACHE_TTL) {
        starsPromise = Promise.resolve(n)
        return starsPromise
      }
    }
  } catch { /* 忽略损坏缓存 */ }
  starsPromise = fetch(`https://api.github.com/repos/${REPO}`, { signal: AbortSignal.timeout(5000) })
    .then((r) => (r.ok ? r.json() : null))
    .then((d: { stargazers_count?: number } | null) => {
      const n = typeof d?.stargazers_count === 'number' ? d.stargazers_count : null
      if (n !== null) {
        try { sessionStorage.setItem(CACHE_KEY, JSON.stringify({ n, t: Date.now() })) } catch { /* 忽略 */ }
      }
      return n
    })
    .catch(() => null)
  return starsPromise
}

function fmtStars(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : String(n)
}

function useStars(): number | null {
  const [stars, setStars] = useState<number | null>(null)
  useEffect(() => {
    let alive = true
    loadStars().then((n) => { if (alive) setStars(n) })
    return () => { alive = false }
  }, [])
  return stars
}

function GitHubMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className={className} aria-hidden="true">
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}

function StarGlyph() {
  return (
    <svg viewBox="0 0 24 24" fill="#e3b341" aria-hidden="true" className="h-3 w-3">
      <path d="M12 2.5l2.9 6.2 6.6.8-4.9 4.6 1.3 6.6L12 17.5l-5.9 3.2 1.3-6.6L2.5 9.5l6.6-.8z" />
    </svg>
  )
}

/** 顶栏徽章：GitHub logo + star 数（粗边硬阴影；新标签打开仓库） */
export function GitHubNavBadge() {
  const stars = useStars()
  return (
    <a
      href={REPO_URL}
      target="_blank"
      rel="noreferrer"
      data-role="github-badge"
      title={`${REPO} · GitHub 开源`}
      className="flex h-9 items-center gap-1.5 rounded-lg border-3 border-foreground bg-background px-2.5 text-[13px] font-bold shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)]"
    >
      <GitHubMark className="h-4 w-4" />
      {stars !== null && (
        <span className="flex items-center gap-1">
          <StarGlyph />
          <span className="font-mono text-xs">{fmtStars(stars)}</span>
        </span>
      )}
    </a>
  )
}

/** 页脚链接：GitHub 仓库 + star（文字流内） */
export function GitHubFooterLink() {
  const stars = useStars()
  return (
    <a
      href={REPO_URL}
      target="_blank"
      rel="noreferrer"
      data-role="github-footer"
      className="inline-flex items-center gap-1.5 hover:text-foreground hover:underline"
    >
      <GitHubMark className="h-3.5 w-3.5" />
      {REPO}
      {stars !== null && (
        <span className="inline-flex items-center gap-1">
          <StarGlyph />
          {fmtStars(stars)}
        </span>
      )}
    </a>
  )
}
