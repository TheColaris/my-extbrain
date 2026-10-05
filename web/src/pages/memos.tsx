import { useEffect, useState } from 'react'
import { Pencil, Pin, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
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

export function MemosPage() {
  const [memos, setMemos] = useState<Memo[] | null>(null)
  const [content, setContent] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [editID, setEditID] = useState<number | null>(null)
  const [editDraft, setEditDraft] = useState('')

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

  // 便签操作按钮：定尺方钮（不随行高拉伸）；悬停显隐见容器（触屏常显）
  const MEMO_BTN = 'bk-interactive flex h-[26px] w-[26px] shrink-0 cursor-pointer items-center justify-center rounded-md border-2 border-foreground bg-card hover:shadow-[2px_2px_0px_var(--shadow-color)]'

  const MemoRow = ({ m }: { m: Memo }) => (
    <div className="group flex items-start gap-3 border-b border-black/15 py-2.5">
      <span className="w-11 shrink-0 pt-0.5 text-right font-mono text-[11px] font-semibold text-muted-foreground">{hm(m.create_time)}</span>
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
        onDoubleClick={() => { setEditID(m.id); setEditDraft(m.content) }}
        dangerouslySetInnerHTML={{ __html: renderMD(m.content, { breaks: true }) }}
      />
      )}
      <div className="flex shrink-0 items-start gap-1 transition-opacity can-hover:opacity-0 can-hover:group-hover:opacity-100 can-hover:group-focus-within:opacity-100">
        <button type="button" aria-label={m.is_pinned ? '取消置顶' : '置顶'} onClick={() => togglePin(m)} className={cn(MEMO_BTN, m.is_pinned && 'bg-primary')}>
          <Pin className="h-[13px] w-[13px]" />
        </button>
        <button type="button" aria-label="编辑" onClick={() => { setEditID(m.id); setEditDraft(m.content) }} className={MEMO_BTN}>
          <Pencil className="h-[13px] w-[13px]" />
        </button>
        <button type="button" aria-label="删除" onClick={() => remove(m.id)} className={cn(MEMO_BTN, 'bg-destructive')}>
          <Trash2 className="h-[13px] w-[13px]" />
        </button>
      </div>
    </div>
  )

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
        <div className="anim-fade-up border-3 border-dashed border-foreground/40 py-8 text-center text-sm font-semibold text-muted-foreground">
          还没有便签
        </div>
      )}
    </div>
  )
}
