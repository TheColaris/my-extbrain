import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { ApiError, searchApi, type MemoSearchHit, type NoteSearchHit, type TodoSearchHit } from '@/lib/api'
import { SEARCH_SOURCE } from '@/lib/enums'
import { cn } from '@/lib/utils'

// 全局搜索页：
// 本页大输入框接管（其他页面顶栏为 ⌘K 触发按钮）；三域分组 + 黄底高亮 +
// 空输入（最近搜索+热门标签）/ 加载骨架 / 空态（幽灵新建笔记）。

type Scope = 'all' | 'todo' | 'memo' | 'note'
type Result = { todos: TodoSearchHit[]; memos: MemoSearchHit[]; notes: NoteSearchHit[] }
const EMPTY: Result = { todos: [], memos: [], notes: [] }
const RECENT_KEY = 'extbrain_recent_searches'

const SCOPES: { key: Scope; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'todo', label: '待办' },
  { key: 'memo', label: '便签' },
  { key: 'note', label: '知识库' },
]

export function SearchPage() {
  const nav = useNavigate()
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') ?? '')
  const [scope, setScope] = useState<Scope>('all')
  const [phase, setPhase] = useState<'blank' | 'loading' | 'done' | 'error'>('blank')
  const [res, setRes] = useState<Result>(EMPTY)
  const [errMsg, setErrMsg] = useState('')
  const [recent, setRecent] = useState<string[]>(() => {
    try { return JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]') as string[] } catch { return [] }
  })
  const abortRef = useRef<AbortController | null>(null)

  const total = res.todos.length + res.memos.length + res.notes.length

  const openNote = (path: string, repo?: string) => {
    const next = new URLSearchParams(sp)
    next.set('note', path)
    if (repo) next.set('repo', repo)
    setSp(next)
  }

  // 防抖 300ms 搜索；空输入回 blank 态
  useEffect(() => {
    const query = q.trim()
    if (!query) {
      abortRef.current?.abort()
      setPhase('blank')
      setRes(EMPTY)
      return
    }
    setPhase('loading')
    const timer = setTimeout(async () => {
      abortRef.current?.abort()
      const ac = new AbortController()
      abortRef.current = ac
      try {
        const r = await searchApi.all(query, ac.signal)
        setRes(r)
        setPhase('done')
        setRecent((prev) => {
          const next = [query, ...prev.filter((x) => x !== query)].slice(0, 8)
          localStorage.setItem(RECENT_KEY, JSON.stringify(next))
          return next
        })
      } catch (e) {
        if (e instanceof DOMException && e.name === 'AbortError') return
        setErrMsg(e instanceof ApiError ? e.message : '搜索失败，请稍后重试')
        setPhase('error')
      }
    }, 300)
    return () => clearTimeout(timer)
  }, [q])

  const counts: Record<Scope, number> = {
    all: total, todo: res.todos.length, memo: res.memos.length, note: res.notes.length,
  }
  const hotTags = useMemo(() => {
    const freq = new Map<string, number>()
    for (const t of res.todos) for (const tag of t.tags ?? []) freq.set(tag, (freq.get(tag) ?? 0) + 1)
    for (const m of res.memos) for (const tag of m.tags ?? []) freq.set(tag, (freq.get(tag) ?? 0) + 1)
    return [...freq.entries()].sort((a, b) => b[1] - a[1]).slice(0, 6)
  }, [res])

  return (
    <div className="mx-auto w-full max-w-5xl px-6 py-8">
      {/* 大输入框 */}
      <div className="flex h-12 items-center gap-3 rounded-xl border-3 border-foreground bg-card px-4 shadow-[3px_3px_0px_var(--shadow-color)]">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" className="h-[18px] w-[18px] shrink-0">
          <circle cx="11" cy="11" r="7" /><path d="m20 20-3.8-3.8" />
        </svg>
        <input
          autoFocus
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setQ('')
          }}
          placeholder="搜索待办、便签、知识库…"
          className="min-w-0 flex-1 bg-transparent text-[15px] font-semibold outline-none"
        />
        {q && (
          <button
            type="button" aria-label="清空"
            onClick={() => setQ('')}
            className="flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-full border-2 border-foreground bg-background"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" className="h-2.5 w-2.5">
              <path d="M6 6l12 12M18 6 6 18" />
            </svg>
          </button>
        )}
        <span className="hidden shrink-0 rounded border-2 border-foreground bg-card px-1.5 font-mono text-[11px] font-semibold shadow-[2px_2px_0px_var(--shadow-color)] sm:block">esc</span>
      </div>

      {/* 范围筛选 */}
      <div className="mt-4 flex flex-wrap gap-2">
        {SCOPES.map((sc) => (
          <button
            key={sc.key} type="button"
            onClick={() => setScope(sc.key)}
            className={cn(
              'rounded-lg border-3 border-foreground px-3.5 py-1.5 text-[13px] font-bold shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)]',
              scope === sc.key ? 'bg-primary' : 'bg-card',
            )}
          >
            {sc.label} <span className={cn('text-[11px] font-semibold', scope === sc.key ? 'text-foreground/70' : 'text-muted-foreground')}>{phase === 'done' ? counts[sc.key] : ''}</span>
          </button>
        ))}
      </div>

      {/* 空输入态 */}
      {phase === 'blank' && (
        <div className="mt-8">
          {recent.length > 0 && (
            <>
              <GroupLabel label="最近搜索" />
              <div className="flex flex-wrap gap-2">
                {recent.map((r) => (
                  <button key={r} type="button" onClick={() => setQ(r)} className="rounded-lg border-3 border-foreground bg-card px-3.5 py-1.5 text-[13px] font-bold shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)]">{r}</button>
                ))}
              </div>
            </>
          )}
          <GroupLabel label="热门标签" />
          <div className="flex flex-wrap gap-2">
            {hotTags.length === 0 && <span className="text-[13px] font-semibold text-muted-foreground">暂无（使用中会自动聚合）</span>}
            {hotTags.map(([tag, n]) => (
              <button key={tag} type="button" onClick={() => setQ(tag)} className="rounded-lg border-3 border-foreground bg-card px-3.5 py-1.5 text-[13px] font-bold shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)]">
                {tag} <span className="text-[11px] font-semibold text-muted-foreground">{n}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* 加载骨架 */}
      {phase === 'loading' && (
        <div className="mt-8">
          <GroupLabel label="搜索中…" />
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="flex gap-3 border-b-2 border-foreground/10 py-3">
              <div className="min-w-0 flex-1">
                <div className="h-3 w-[60%] rounded bg-foreground/10" />
                <div className="mt-2 h-3 w-[30%] rounded bg-foreground/10" />
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 错误态 */}
      {phase === 'error' && (
        <div className="mt-8 rounded-xl border-3 border-foreground border-l-[7px] border-l-[var(--neon-red,var(--neon-pink))] bg-card p-6 text-center shadow-[4px_4px_0px_var(--shadow-color)]">
          <div className="text-[15px] font-extrabold">搜索出错</div>
          <div className="mt-1.5 text-[13px] font-semibold text-muted-foreground">{errMsg}</div>
        </div>
      )}

      {/* 结果 */}
      {phase === 'done' && (
        <>
          {total === 0 ? (
            <div className="mt-8 rounded-xl border-[3px] border-dashed border-foreground bg-white/45 p-10 text-center">
              <div className="text-[15px] font-extrabold">没有匹配的结果</div>
              <div className="mt-1.5 text-[13px] font-semibold text-muted-foreground">换个词，或者把它记下来</div>
              <button
                type="button"
                onClick={() => nav('/notes')}
                className="mt-4 inline-flex items-center gap-2 rounded-lg border-3 border-foreground bg-primary px-4 py-2 font-mono text-[13px] font-extrabold shadow-[3px_3px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[5px_5px_0px_var(--shadow-color)]"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" className="h-3.5 w-3.5"><path d="M12 5v14M5 12h14" /></svg>
                新建笔记 {q.trim()}.md
              </button>
            </div>
          ) : (
            <>
              {(scope === 'all' || scope === 'todo') && res.todos.length > 0 && (
                <Section icon={<CheckIcon />} tone="bg-card" label="待办" cnt={res.todos.length}
                  onMore={scope === 'all' ? () => setScope('todo') : undefined}>
                  {res.todos.map((t) => (
                    <div key={t.id} className="group flex items-start gap-3 border-b-2 border-foreground/10 py-3">
                      <span className={cn('mt-0.5 h-[18px] w-[18px] shrink-0 rounded-full border-[2.5px] border-foreground', t.status === 'done' ? 'bg-[var(--neon-green)]' : 'bg-background')} />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2 text-[13.5px] font-bold leading-relaxed">
                          <Highlight text={t.title} q={q.trim()} />
                          {t.due_time && <DueBadge due={t.due_time} done={t.status === 'done'} />}
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-2 text-[11.5px] font-semibold text-muted-foreground">
                          <span className="inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-[var(--neon-green)] px-1 text-[10.5px] font-bold text-foreground">
                            {t.source === 'cli' ? `CLI${t.api_key_id ? '' : ''}` : 'Web'}
                          </span>
                          {(t.tags ?? []).map((tag) => <TagChip key={tag} tag={tag} />)}
                          <span>创建于 {fmtDay(t.create_time)}</span>
                        </div>
                        {t.snippet && <p className="mt-0.5 text-[12.5px] font-medium leading-relaxed text-muted-foreground"><Highlight text={t.snippet} q={q.trim()} /></p>}
                      </div>
                      <div className="mt-0.5 flex shrink-0 gap-1 opacity-0 transition-opacity duration-150 focus-within:opacity-100 group-hover:opacity-100">
                        {t.status !== 'done' && (
                          <span className="flex h-7 w-7 items-center justify-center rounded-lg border-2 border-foreground bg-[var(--neon-green)]" title="在待办页完成">
                            <CheckIcon />
                          </span>
                        )}
                      </div>
                    </div>
                  ))}
                </Section>
              )}

              {(scope === 'all' || scope === 'memo') && res.memos.length > 0 && (
                <Section icon={<MemoIcon />} tone="bg-[var(--neon-blue)]" label="便签" cnt={res.memos.length}
                  onMore={scope === 'all' ? () => setScope('memo') : undefined}>
                  {res.memos.map((m) => (
                    <div key={m.id} className="group flex items-start gap-3 border-b-2 border-foreground/10 py-3">
                      <div className="min-w-0 flex-1">
                        <p className="line-clamp-2 text-[13px] font-medium leading-relaxed text-foreground/85 [overflow-wrap:anywhere]"><Highlight text={m.snippet} q={q.trim()} /></p>
                        <div className="mt-1.5 flex flex-wrap items-center gap-2 text-[11.5px] font-semibold text-muted-foreground">
                          <span>{fmtDay(m.create_time)}</span>
                          {(m.tags ?? []).map((tag) => <TagChip key={tag} tag={tag} />)}
                        </div>
                      </div>
                    </div>
                  ))}
                </Section>
              )}

              {(scope === 'all' || scope === 'note') && res.notes.length > 0 && (
                <Section icon={<NoteIcon />} tone="bg-primary" label="知识库" cnt={res.notes.length}
                  onMore={scope === 'all' ? () => setScope('note') : undefined}>
                  {res.notes.map((n) => (
                    <div
                      key={n.path} role="button" tabIndex={0}
                      onClick={() => openNote(n.path, n.repo_name)}
                      onKeyDown={(e) => e.key === 'Enter' && openNote(n.path, n.repo_name)}
                      className="group flex cursor-pointer items-start gap-3 border-b-2 border-foreground/10 py-3"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="font-mono text-[12.5px] font-bold leading-relaxed [overflow-wrap:anywhere]">
                          {n.repo_name && (
                            <span
                              className="mr-1.5 inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-[var(--neon-blue)] px-1 align-middle text-[10.5px] font-bold"
                              data-role="hit-repo"
                              title="所属仓库"
                            >
                              {n.repo_name}
                            </span>
                          )}
                          <Highlight text={n.path} q={q.trim()} />
                          {(n.source === SEARCH_SOURCE.semantic || n.source === SEARCH_SOURCE.both) && (
                            <span
                              className="ml-1.5 inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-[var(--neon-purple)] px-1 align-middle text-[10.5px] font-bold"
                              title="语义命中"
                              data-role="sem-badge"
                            >
                              语义
                            </span>
                          )}
                        </div>
                        <p className="mt-0.5 line-clamp-2 text-[12.5px] font-medium leading-relaxed text-muted-foreground [overflow-wrap:anywhere]"><Highlight text={n.snippet} q={q.trim()} /></p>
                        <div className="mt-1.5 flex flex-wrap items-center gap-2 text-[11.5px] font-semibold text-muted-foreground">
                          <span>更新 {fmtDay(n.updated_at)}</span>
                          <span>{fmtSize(n.size_bytes)}</span>
                        </div>
                      </div>
                      <div className="mt-0.5 flex shrink-0 gap-1 opacity-0 transition-opacity duration-150 focus-within:opacity-100 group-hover:opacity-100">
                        <span className="flex h-7 w-7 items-center justify-center rounded-lg border-2 border-foreground bg-card" title="打开">
                          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" className="h-3.5 w-3.5"><path d="M7 17 17 7" /><path d="M9 7h8v8" /></svg>
                        </span>
                      </div>
                    </div>
                  ))}
                </Section>
              )}

              <div className="mt-5 flex flex-wrap items-center justify-between gap-2.5">
                <span className="text-[12px] font-semibold text-muted-foreground">
                  {counts.all} 条结果 · {scope === 'all' ? '全部范围' : SCOPES.find((s2) => s2.key === scope)?.label + '范围'}
                </span>
                <div className="hidden items-center gap-3 text-[11.5px] font-semibold text-muted-foreground sm:flex">
                  <span className="flex items-center gap-1"><Kbd>↑</Kbd><Kbd>↓</Kbd> 选择</span>
                  <span className="flex items-center gap-1"><Kbd>esc</Kbd> 清空</span>
                </div>
              </div>
            </>
          )}
        </>
      )}
    </div>
  )
}

function GroupLabel({ label }: { label: string }) {
  return (
    <div className="mb-2.5 mt-7 flex items-center gap-2 text-[12px] font-extrabold uppercase tracking-wider text-muted-foreground">
      <span className="h-2.5 w-2.5 rounded border-2 border-foreground bg-[var(--neon-purple)]" />
      {label}
    </div>
  )
}

function Section({ icon, tone, label, cnt, onMore, children }: {
  icon: React.ReactNode; tone: string; label: string; cnt: number; onMore?: () => void; children: React.ReactNode
}) {
  return (
    <section>
      <div className="mb-1 mt-7 flex items-center gap-2.5">
        <span className={cn('flex h-6 w-6 items-center justify-center rounded-md border-2 border-foreground', tone)}>{icon}</span>
        <span className="text-[14px] font-extrabold">{label}</span>
        <span className="text-[12px] font-bold text-muted-foreground">{cnt}</span>
        {onMore && (
          <button type="button" onClick={onMore} className="ml-auto rounded-md border-2 border-transparent px-2 py-0.5 text-[12px] font-bold text-muted-foreground hover:border-foreground hover:bg-card hover:shadow-[2px_2px_0px_var(--shadow-color)]">
            查看全部 →
          </button>
        )}
      </div>
      <div>{children}</div>
    </section>
  )
}

/** 关键词黄底高亮（大小写不敏感切分，纯 React 节点无注入面） */
function Highlight({ text, q }: { text: string; q: string }) {
  if (!q) return <>{text}</>
  const parts: (string | { hit: string })[] = []
  const lower = text.toLowerCase()
  const lq = q.toLowerCase()
  let i = 0
  while (i < text.length) {
    const hit = lower.indexOf(lq, i)
    if (hit < 0) { parts.push(text.slice(i)); break }
    if (hit > i) parts.push(text.slice(i, hit))
    parts.push({ hit: text.slice(hit, hit + q.length) })
    i = hit + q.length
  }
  return (
    <>
      {parts.map((p, idx) =>
        typeof p === 'string' ? <span key={idx}>{p}</span> : (
          <mark key={idx} className="rounded bg-primary px-0.5 font-extrabold text-foreground">{p.hit}</mark>
        ),
      )}
    </>
  )
}

function DueBadge({ due, done }: { due: string; done: boolean }) {
  const d = new Date(due)
  const overdue = !done && d.getTime() < Date.now()
  const isToday = new Date().toDateString() === d.toDateString()
  if (done) return null
  return (
    <span className={cn('inline-flex h-[19px] shrink-0 items-center rounded-md border-2 border-foreground px-1.5 text-[11px] font-extrabold', overdue ? 'bg-[var(--neon-red)]' : isToday ? 'bg-primary' : 'bg-card')}>
      {overdue ? '逾期 ' : isToday ? '今天 ' : ''}{pad2(d.getMonth() + 1)}-{pad2(d.getDate())} {pad2(d.getHours())}:{pad2(d.getMinutes())}
    </span>
  )
}

function TagChip({ tag }: { tag: string }) {
  return <span className="inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-background px-1 text-[10.5px] font-bold">{tag}</span>
}

function Kbd({ children }: { children: React.ReactNode }) {
  return <span className="rounded border-2 border-foreground bg-card px-1.5 font-mono text-[10px] font-semibold shadow-[2px_2px_0px_var(--shadow-color)]">{children}</span>
}

const CheckIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className="h-3 w-3"><path d="M4 12.5 9.5 18 20 6" /></svg>
)
const MemoIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-3 w-3"><path d="M4 5h16v11l-4 4H4z" /><path d="M16 20v-4h4" /></svg>
)
const NoteIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-3 w-3"><path d="M4 4h7v16H4z" /><path d="M11 4h9v16h-9" /><path d="M14 9h3M14 13h3" /></svg>
)

function fmtDay(iso: string): string {
  const d = new Date(iso)
  return `${d.getMonth() + 1}-${String(d.getDate()).padStart(2, '0')}`
}
function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`
  return `${(n / 1024).toFixed(1)} KB`
}
function pad2(n: number): string {
  return String(n).padStart(2, '0')
}
