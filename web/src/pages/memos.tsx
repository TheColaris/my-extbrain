import { useEffect, useRef, useState } from 'react'
import { ChevronDown, MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Textarea } from '@/components/ui/textarea'
import { EmptyDemo } from '@/components/empty-demo'
import { ApiError, memosApi, type Memo } from '@/lib/api'
import { renderMD } from '@/lib/md'
import { cn } from '@/lib/utils'

function dayOf(iso: string): string {
  const d = new Date(iso)
  const now = new Date()
  const y = new Date(now); y.setDate(now.getDate() - 1)
  if (d.toDateString() === now.toDateString()) return '今天'
  if (d.toDateString() === y.toDateString()) return '昨天'
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日`
}
function hm(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}`
}

// 移动端折叠视图（<768px）：列表态只显摘要，点行展开全文；桌面恒全量展开
function useCompactMode(): boolean {
  const [compact, setCompact] = useState(() => window.matchMedia('(max-width: 767px)').matches)
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 767px)')
    const fn = (e: MediaQueryListEvent) => setCompact(e.matches)
    mq.addEventListener('change', fn)
    return () => mq.removeEventListener('change', fn)
  }, [])
  return compact
}

// 去 MD 语法取纯文本预览（折叠态摘要）
function plainPreview(s: string): string {
  return s
    .replace(/```[\s\S]*?```/g, ' [代码] ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]*)\]\(([^)]*)\)/g, '$1')
    .replace(/[#>*_`~|]/g, '')
    .replace(/^\s*[-+]\s+/gm, '· ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 160)
}

export function MemosPage() {
  const [memos, setMemos] = useState<Memo[] | null>(null)
  const [content, setContent] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [editID, setEditID] = useState<number | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const compact = useCompactMode()
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  const toggleExpand = (id: number) => setExpanded((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })

  const saveEdit = async (id: number) => {
    if (!editDraft.trim()) return
    try {
      await memosApi.update(id, { content: editDraft.trim() })
      setEditID(null)
      reload()
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '保存失败')
    }
  }

  const reload = () => memosApi.list().then((r) => setMemos(r.memos)).catch(() => setMemos([]))
  useEffect(() => { reload() }, [])

  async function add() {
    if (!content.trim()) { setErr('一句话也值得记'); return }
    setErr(''); setBusy(true)
    try {
      await memosApi.create({ content: content.trim() })
      setContent('')
      reload()
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '记录失败')
    } finally {
      setBusy(false)
    }
  }

  async function togglePin(m: Memo) {
    await memosApi.update(m.id, { is_pinned: m.is_pinned ? 0 : 1 }).catch(() => {})
    reload()
  }
  async function remove(id: number) {
    await memosApi.del(id).catch(() => {})
    reload()
  }

  // 分组：置顶 / 按天
  const pinned = (memos ?? []).filter((m) => m.is_pinned)
  const byDay = new Map<string, Memo[]>()
  ;(memos ?? []).filter((m) => !m.is_pinned).forEach((m) => {
    const k = dayOf(m.create_time)
    byDay.set(k, [...(byDay.get(k) ?? []), m])
  })

  // 便签操作钮：单钮收拢三操作（定尺方钮，不随行高拉伸）；触屏常显、桌面悬停显
  const MEMO_BTN = 'bk-interactive flex h-[26px] w-[26px] shrink-0 cursor-pointer items-center justify-center rounded-md border-2 border-foreground bg-card hover:shadow-[2px_2px_0px_var(--shadow-color)]'

  const MemoRow = ({ m }: { m: Memo }) => {
    const clickTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
    const clearTimer = () => { if (clickTimer.current) { clearTimeout(clickTimer.current); clickTimer.current = null } }
    // 单击延迟 toggle（200ms 给双击编辑让路：双击=清 timer 直接进编辑）
    const delayedToggle = () => { clearTimer(); clickTimer.current = setTimeout(() => { toggleExpand(m.id); clickTimer.current = null }, 200) }
    const openEdit = () => { clearTimer(); setEditID(m.id); setEditDraft(m.content) }
    const collapsed = compact && !expanded.has(m.id) && editID !== m.id

    if (collapsed) {
      return (
        <div className="flex cursor-pointer items-center gap-2 border-b border-black/15 py-2 sm:gap-3 sm:py-2.5" onClick={delayedToggle} onDoubleClick={openEdit}>
          <span className="w-9 shrink-0 text-right font-mono text-[11px] font-semibold text-muted-foreground sm:w-11">{hm(m.create_time)}</span>
          <p className="min-w-0 flex-1 line-clamp-2 text-[13px] leading-snug text-foreground/85">{plainPreview(m.content)}</p>
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        </div>
      )
    }

    return (
    <div className="group flex items-start gap-2 border-b border-black/15 py-2.5 sm:gap-3">
      <span className="w-9 shrink-0 pt-0.5 text-right font-mono text-[11px] font-semibold text-muted-foreground sm:w-11">{hm(m.create_time)}</span>
      {editID === m.id ? (
        <div className="min-w-0 flex-1 space-y-2">
          <Textarea
            autoFocus
            rows={2}
            value={editDraft}
            onChange={(e) => setEditDraft(e.target.value)}
            className="resize-none text-sm"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) saveEdit(m.id)
              if (e.key === 'Escape') setEditID(null)
            }}
          />
          <div className="flex justify-end gap-2">
            <Button size="sm" variant="outline" onClick={() => setEditID(null)}>取消</Button>
            <Button size="sm" onClick={() => saveEdit(m.id)}>保存</Button>
          </div>
        </div>
      ) : (
      <div
        className="markdown-body md-memo min-w-0 flex-1 cursor-text"
        title="双击编辑"
        onClick={compact ? delayedToggle : undefined}
        onDoubleClick={openEdit}
        dangerouslySetInnerHTML={{ __html: renderMD(m.content, { breaks: true }) }}
      />
      )}
      {/* 操作收拢为单钮菜单：正文列让位（悬停显隐见容器，触屏常显单钮） */}
      <div className="flex shrink-0 items-start transition-opacity can-hover:opacity-0 can-hover:group-hover:opacity-100 can-hover:group-focus-within:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button type="button" aria-label="便签操作" className={cn(MEMO_BTN, m.is_pinned && 'bg-primary')}>
              <MoreHorizontal className="h-[13px] w-[13px]" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-28">
            <DropdownMenuItem onClick={() => togglePin(m)}>{m.is_pinned ? '取消置顶' : '置顶'}</DropdownMenuItem>
            <DropdownMenuItem onClick={() => { setEditID(m.id); setEditDraft(m.content) }}>编辑</DropdownMenuItem>
            <DropdownMenuItem onClick={() => remove(m.id)} className="text-destructive focus:text-destructive">删除</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div className="anim-fade-up">
        <h1 className="text-xl font-extrabold tracking-tight">便签</h1>
        <p className="mt-0.5 text-xs font-semibold text-muted-foreground">想到就记 · 超过 2000 字转知识库</p>
      </div>

      {/* 输入卡 */}
      <div className="anim-fade-up rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
        <Textarea
          rows={2}
          placeholder="一句话也值得记…"
          value={content}
          onChange={(e) => setContent(e.target.value)}
          className="resize-none border-none shadow-none focus-visible:ring-0"
        />
        <div className="mt-2 flex items-center justify-between">
          <span className={cn('text-xs font-bold', content.length > 2000 ? 'text-destructive' : 'text-muted-foreground')}>
            {content.length > 1800 ? `${content.length}/2000` : ''}
          </span>
          <Button size="sm" onClick={add} disabled={busy}>{busy ? <span className="bk-loader" /> : '记录'}</Button>
        </div>
      </div>
      {err && <p className="anim-pop border-3 border-foreground bg-destructive px-3 py-2 text-sm font-bold text-destructive-foreground">{err}</p>}

      {pinned.length > 0 && (
        <section className="anim-fade-up">
          <div className="mb-1 text-xs font-extrabold uppercase tracking-wider text-muted-foreground">置顶</div>
          <div className="border-t-2 border-foreground">
            {pinned.map((m) => <MemoRow key={m.id} m={m} />)}
          </div>
        </section>
      )}

      {[...byDay.entries()].map(([day, list]) => (
        <section key={day} className="anim-fade-up">
          <div className="mb-1 text-xs font-extrabold uppercase tracking-wider text-muted-foreground">{day}</div>
          <div className="border-t-2 border-foreground">
            {list.map((m) => <MemoRow key={m.id} m={m} />)}
          </div>
        </section>
      ))}

      {memos?.length === 0 && (
        <div className="anim-fade-up border-3 border-dashed border-foreground bg-card/60 px-6 py-10 text-center">
          <div className="text-base font-extrabold">还没有便签</div>
          <p className="mt-1.5 text-sm font-semibold text-muted-foreground">想到就记，一句话也值得；或对 AI 说一句：</p>
          <EmptyDemo say="记一下：GLM-5.3 tool call 长上下文会丢参数名">
            <div className="flex items-center gap-2.5 rounded-lg border-2 border-foreground bg-card px-3 py-2.5 text-left text-[13px] font-semibold shadow-[3px_3px_0px_var(--shadow-color)]">
              <span className="shrink-0 font-mono text-[11px] font-semibold text-muted-foreground">14:32</span>
              <span className="min-w-0 flex-1">GLM-5.3 tool call 长上下文会丢参数名，改用 JSON schema 严格模式</span>
            </div>
          </EmptyDemo>
        </div>
      )}
    </div>
  )
}
