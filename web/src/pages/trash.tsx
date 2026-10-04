import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ApiError, trashApi, type TrashItem } from '@/lib/api'
import { TRASH_TYPE, type TrashType } from '@/lib/enums'
import { cn, relTime } from '@/lib/utils'

// 回收站：
// 三实体软删统一找回——行内回答 谁删的（来源）/ 还剩几天 / 原信息；
// 恢复（撤销 toast）/ 彻底删除（行内+批量+清空）。

const SCOPES: { key: TrashType | 'all'; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'todo', label: '待办' },
  { key: 'memo', label: '便签' },
  { key: 'note', label: '知识库' },
]

export function TrashPage() {
  const [items, setItems] = useState<TrashItem[]>([])
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [scope, setScope] = useState<TrashType | 'all'>('all')
  const [filter, setFilter] = useState('')
  const [loading, setLoading] = useState(true)
  const [errMsg, setErrMsg] = useState('')
  const [busyId, setBusyId] = useState<string>('')
  const [checked, setChecked] = useState<Set<string>>(new Set())
  const [purgeOpen, setPurgeOpen] = useState(false)

  const load = async () => {
    setLoading(true)
    setErrMsg('')
    try {
      const r = await trashApi.list()
      setItems(r.items)
      setCounts(r.counts)
    } catch (e) {
      setErrMsg(e instanceof ApiError ? e.message : '加载失败，请稍后重试')
    } finally {
      setLoading(false)
    }
  }
  useEffect(() => { void load() }, [])

  const shown = useMemo(() => {
    let list = scope === 'all' ? items : items.filter((it) => it.type === scope)
    const f = filter.trim().toLowerCase()
    if (f) list = list.filter((it) => (it.title + ' ' + (it.subtitle ?? '') + ' ' + (it.tags ?? []).join(' ')).toLowerCase().includes(f))
    return list
  }, [items, scope, filter])

  const keyOf = (it: TrashItem) => `${it.type}:${it.id}`
  const checkedItems = items.filter((it) => checked.has(keyOf(it)))

  const toggle = (it: TrashItem) => {
    setChecked((prev) => {
      const next = new Set(prev)
      const k = keyOf(it)
      if (next.has(k)) next.delete(k); else next.add(k)
      return next
    })
  }

  const doRestore = async (it: TrashItem) => {
    setBusyId(keyOf(it))
    try {
      await trashApi.restore(it.type, it.id)
      setItems((prev) => prev.filter((x) => keyOf(x) !== keyOf(it)))
      setCounts((prev) => ({ ...prev, [it.type]: Math.max(0, (prev[it.type] ?? 1) - 1) }))
      setChecked((prev) => { const n = new Set(prev); n.delete(keyOf(it)); return n })
    } catch (e) {
      setErrMsg(e instanceof ApiError ? e.message : '恢复失败')
    } finally {
      setBusyId('')
    }
  }

  const doPurgeItem = async (it: TrashItem) => {
    setBusyId(keyOf(it))
    try {
      await trashApi.purgeItem(it.type, it.id)
      setItems((prev) => prev.filter((x) => keyOf(x) !== keyOf(it)))
      setCounts((prev) => ({ ...prev, [it.type]: Math.max(0, (prev[it.type] ?? 1) - 1) }))
      setChecked((prev) => { const n = new Set(prev); n.delete(keyOf(it)); return n })
    } catch (e) {
      setErrMsg(e instanceof ApiError ? e.message : '删除失败')
    } finally {
      setBusyId('')
    }
  }

  const doBatch = async (mode: 'restore' | 'purge') => {
    for (const it of checkedItems) {
      try {
        if (mode === 'restore') await trashApi.restore(it.type, it.id)
        else await trashApi.purgeItem(it.type, it.id)
      } catch { /* 单条失败不阻断批次；结束后统一刷新 */ }
    }
    setChecked(new Set())
    await load()
  }

  const doPurgeAll = async () => {
    setPurgeOpen(false)
    try {
      await trashApi.purgeAll()
      setChecked(new Set())
      await load()
    } catch (e) {
      setErrMsg(e instanceof ApiError ? e.message : '清空失败')
    }
  }

  return (
    <div className="mx-auto w-full max-w-6xl px-6 py-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-[20px] font-extrabold tracking-tight">回收站</h1>
          <p className="mt-0.5 text-[12px] font-medium text-muted-foreground">软删除 · 保留 30 天 · 谁删的、还能恢复几天，都在行上</p>
        </div>
        <Button variant="destructive" size="sm" disabled={items.length === 0} onClick={() => setPurgeOpen(true)}>清空回收站</Button>
      </div>

      {/* 工具行 */}
      <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-2">
          {SCOPES.map((sc) => (
            <button
              key={sc.key} type="button"
              onClick={() => setScope(sc.key)}
              className={cn(
                'rounded-lg border-3 border-foreground px-3.5 py-1.5 text-[13px] font-bold shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)]',
                scope === sc.key ? 'bg-primary' : 'bg-card',
              )}
            >
              {sc.label} <span className="text-[11px] font-semibold text-muted-foreground">{sc.key === 'all' ? items.length : counts[sc.key] ?? 0}</span>
            </button>
          ))}
        </div>
        <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="筛选回收站内容…" className="h-8 w-full text-[12.5px] sm:w-56" />
      </div>

      {/* 列表 */}
      {loading ? (
        <div className="mt-4">
          {[0, 1, 2].map((i) => (
            <div key={i} className="border-b-2 border-foreground/10 py-3.5">
              <div className="h-3 w-[45%] rounded bg-foreground/10" />
              <div className="mt-2 h-3 w-[25%] rounded bg-foreground/10" />
            </div>
          ))}
        </div>
      ) : errMsg && items.length === 0 ? (
        <div className="mt-6 rounded-xl border-3 border-foreground border-l-[7px] border-l-[var(--neon-red)] bg-card p-5 text-[13px] font-bold shadow-[4px_4px_0px_var(--shadow-color)]">{errMsg}</div>
      ) : shown.length === 0 ? (
        <div className="mt-6 rounded-xl border-[3px] border-dashed border-foreground bg-white/45 p-11 text-center">
          <div className="text-[15px] font-extrabold">回收站是空的</div>
          <div className="mt-1.5 text-[13px] font-semibold text-muted-foreground">删除的待办、便签、笔记会在这里保留 30 天</div>
        </div>
      ) : (
        <div className="mt-2">
          {shown.map((it) => {
            const k = keyOf(it)
            const busy = busyId === k
            return (
              <div key={k} className="group flex items-start gap-3 border-b-2 border-foreground/10 py-3">
                <button
                  type="button" aria-label="选中"
                  onClick={() => toggle(it)}
                  className={cn('mt-1 flex h-[18px] w-[18px] shrink-0 cursor-pointer items-center justify-center rounded border-[2.5px] border-foreground bg-background', checked.has(k) && 'bg-[var(--neon-green)]')}
                >
                  {checked.has(k) && (
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3.5" className="h-2.5 w-2.5"><path d="M5 13l4 4L19 7" /></svg>
                  )}
                </button>
                <TypeIcon type={it.type} />
                <div className="min-w-0 flex-1">
                  <div className="text-[13.5px] font-bold leading-relaxed [overflow-wrap:anywhere]">{it.title}</div>
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-[11.5px] font-semibold text-muted-foreground">
                    <SourceChip source={it.source} />
                    {it.type === 'note' && it.repo_name && (
                      <span className="inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-[var(--neon-blue)] px-1 text-[10.5px] font-bold text-foreground" data-role="trash-repo">{it.repo_name}</span>
                    )}
                    {it.subtitle && <span className="[overflow-wrap:anywhere]">{it.subtitle}</span>}
                    {(it.tags ?? []).map((tag) => (
                      <span key={tag} className="inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-background px-1 text-[10.5px] font-bold text-foreground">{tag}</span>
                    ))}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2.5">
                  <span className="whitespace-nowrap font-mono text-[11px] text-muted-foreground">删除于 {relTime(it.deleted_at)}</span>
                  <span className={cn('inline-flex h-[19px] shrink-0 items-center rounded-md border-2 border-foreground px-1.5 text-[11px] font-extrabold', it.left_days <= 7 ? 'bg-[var(--neon-red)]' : 'bg-background')}>
                    剩 {it.left_days} 天
                  </span>
                  <div className="flex gap-1 opacity-0 transition-opacity duration-150 focus-within:opacity-100 group-hover:opacity-100 max-lg:opacity-100">
                    <button type="button" disabled={busy} onClick={() => doRestore(it)} title="恢复"
                      className="flex h-7 w-7 items-center justify-center rounded-lg border-2 border-foreground bg-[var(--neon-green)] disabled:opacity-40">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" className="h-3.5 w-3.5"><path d="M9 14 4 9l5-5" /><path d="M4 9h10a6 6 0 0 1 0 12h-3" /></svg>
                    </button>
                    <button type="button" disabled={busy} onClick={() => doPurgeItem(it)} title="彻底删除"
                      className="flex h-7 w-7 items-center justify-center rounded-lg border-2 border-foreground bg-[var(--neon-red)] disabled:opacity-40">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" className="h-3.5 w-3.5"><path d="M4 7h16" /><path d="M9 7V4h6v3" /><path d="M6 7l1 13h10l1-13" /></svg>
                    </button>
                  </div>
                </div>
              </div>
            )
          })}
          <div className="mt-4 text-[12px] font-semibold text-muted-foreground">
            {items.length} 条 · 最早 {items.length ? relTime(items[items.length - 1].deleted_at).replace(/^删除于 /, '') : ''} 删除 · 到期自动清理
          </div>
        </div>
      )}

      {/* 批量操作条 */}
      {checkedItems.length > 0 && (
        <div className="fixed bottom-6 left-1/2 z-50 flex -translate-x-1/2 items-center gap-3.5 rounded-xl border-3 border-foreground bg-card px-4 py-2.5 shadow-[4px_4px_0px_var(--shadow-color)]">
          <span className="whitespace-nowrap text-[13px] font-extrabold">已选 {checkedItems.length} 项</span>
          <Button size="sm" onClick={() => doBatch('restore')}>批量恢复</Button>
          <Button size="sm" variant="destructive" onClick={() => doBatch('purge')}>彻底删除</Button>
          <button type="button" aria-label="取消选择" onClick={() => setChecked(new Set())} className="flex h-7 w-7 items-center justify-center rounded-lg">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className="h-3.5 w-3.5"><path d="M6 6l12 12M18 6 6 18" /></svg>
          </button>
        </div>
      )}

      {/* 清空确认（红头弹窗，分类型计数） */}
      <Dialog open={purgeOpen} onOpenChange={setPurgeOpen}>
        <DialogContent className="max-w-md gap-0 overflow-hidden rounded-xl border-3 border-foreground p-0 shadow-[6px_6px_0px_var(--shadow-color)] sm:rounded-xl">
          <DialogHeader className="m-0 flex items-center justify-between border-b-3 border-foreground bg-[var(--neon-red)] px-4 py-3">
            <DialogTitle className="text-[15px] font-extrabold">清空回收站</DialogTitle>
          </DialogHeader>
          <div className="px-4 py-5 text-[13.5px] font-semibold leading-relaxed">
            将对 {items.length} 条内容执行<b>彻底删除</b>，不可恢复。<br />
            待办 {counts.todo ?? 0} · 便签 {counts.memo ?? 0} · 笔记 {counts.note ?? 0}。
          </div>
          <div className="flex justify-end gap-2.5 border-t-2 border-foreground bg-background px-4 py-3">
            <Button variant="outline" size="sm" onClick={() => setPurgeOpen(false)}>取消</Button>
            <Button variant="destructive" size="sm" onClick={doPurgeAll}>彻底删除 {items.length} 条</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function TypeIcon({ type }: { type: TrashType }) {
  const cls = 'flex h-[26px] w-[26px] shrink-0 items-center justify-center rounded-md border-2 border-foreground'
  if (type === TRASH_TYPE.todo) {
    return <span className={cn(cls, 'bg-card')}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className="h-3 w-3"><path d="M4 12.5 9.5 18 20 6" /></svg></span>
  }
  if (type === TRASH_TYPE.memo) {
    return <span className={cn(cls, 'bg-[var(--neon-blue)]')}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-3 w-3"><path d="M4 5h16v11l-4 4H4z" /><path d="M16 20v-4h4" /></svg></span>
  }
  return <span className={cn(cls, 'bg-primary')}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-3 w-3"><path d="M4 4h7v16H4z" /><path d="M11 4h9v16h-9" /><path d="M14 9h3M14 13h3" /></svg></span>
}

function SourceChip({ source }: { source: string }) {
  if (!source) return null
  const isWeb = source === 'Web'
  return (
    <span className={cn('inline-flex h-[17px] shrink-0 items-center whitespace-nowrap rounded border-[1.5px] border-foreground px-1 text-[10.5px] font-bold text-foreground', isWeb ? 'bg-background' : 'bg-[var(--neon-green)]')}>
      {source}
    </span>
  )
}
