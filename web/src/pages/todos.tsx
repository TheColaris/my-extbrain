import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { undoToast } from '@/components/undo-toast'
import { ApiError, todosApi, type Todo, type TodoCounts } from '@/lib/api'
import { TODO_SORT, TODO_STATUS, type TodoSort } from '@/lib/enums'
import { cn } from '@/lib/utils'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

/* ================= 工具 ================= */
const pad = (n: number) => String(n).padStart(2, '0')
const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
const fmtMD = (d: Date) => `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
const fmtHM = (d: Date) => `${pad(d.getHours())}:${pad(d.getMinutes())}`
const fmtFull = (d: Date) => `${fmtMD(d)} ${fmtHM(d)}`
function toLocalInput(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
function mdHTML(s: string): string {
  return DOMPurify.sanitize(marked.parse(s, { async: false, breaks: true }) as string)
}

/* 行动画类型：完成（lime 闪+划线）/ 恢复（黄闪）/ 增改定位（黄底闪） */
type AnimKind = 'just-done' | 'just-active' | 'flash'
type Anim = { id: number; kind: AnimKind }

/* 徽章 / 列宽常量（两套：<lg 四列、≥lg 六列） */
const GRID = 'grid grid-cols-[34px_minmax(0,1fr)_118px_66px] lg:grid-cols-[40px_minmax(0,1fr)_152px_132px_64px_70px]'
const CHIP = 'inline-flex items-center border-2 border-foreground px-1.5 py-0.5 text-xs font-bold whitespace-nowrap'
const OPS = 'flex justify-end opacity-100 transition-opacity lg:opacity-0 lg:group-hover:opacity-100'

function DueBadge({ t }: { t: Todo }) {
  if (t.status === TODO_STATUS.done) {
    if (!t.completed_at) return <span className="text-muted-foreground">—</span>
    return <span className={cn(CHIP, 'bg-background')} title="完成时间">✓ {fmtFull(new Date(t.completed_at))}</span>
  }
  if (!t.due_time) return <span className="text-muted-foreground">—</span>
  const d = new Date(t.due_time)
  const n = new Date()
  const overdue = d < n
  const tm = new Date(); tm.setDate(tm.getDate() + 1)
  const yd = new Date(); yd.setDate(yd.getDate() - 1)
  let text: string
  if (sameDay(d, n)) text = `今天 ${fmtHM(d)}`
  else if (sameDay(d, tm)) text = `明天 ${fmtHM(d)}`
  else if (sameDay(d, yd)) text = `昨天 ${fmtHM(d)}`
  else text = `${fmtMD(d)} ${fmtHM(d)}`
  return (
    <span className={cn(CHIP, overdue ? 'bg-destructive text-destructive-foreground' : sameDay(d, n) ? 'bg-[var(--neon-yellow)]' : 'bg-background')} title={overdue ? '已逾期' : undefined}>
      {text}
    </span>
  )
}

function StatusChip({ status }: { status: Todo['status'] }) {
  return (
    <span className={cn(CHIP, status === TODO_STATUS.done ? 'bg-[var(--neon-green)]' : 'bg-background')}>
      {status === TODO_STATUS.done ? '已完成' : '在途'}
    </span>
  )
}

function SourceChip({ source }: { source: Todo['source'] }) {
  return (
    <span className={cn(CHIP, source === 'cli' ? 'bg-[var(--neon-purple)]' : 'bg-[var(--neon-blue)]')}>
      {source === 'cli' ? 'CLI' : 'Web'}
    </span>
  )
}

/* ================= 行 ================= */
function Row({ t, anim, onOpen, onToggle, onDelete }: {
  t: Todo
  anim: Anim | null
  onOpen: (t: Todo) => void
  onToggle: (t: Todo) => void
  onDelete: (t: Todo) => void
}) {
  const isDone = t.status === TODO_STATUS.done
  const mine = anim?.id === t.id ? anim.kind : null
  return (
    <div
      className={cn(GRID, 'group cursor-pointer items-center border-b border-foreground/15 px-4 py-2.5 transition-colors last:border-b-0 hover:bg-primary/10', isDone && 'bg-background/60',
        mine === 'just-done' && 'row-just-done',
        mine === 'just-active' && 'row-just-active',
        mine === 'flash' && 'row-flash')}
      onClick={() => onOpen(t)}
      title="点击查看详情"
    >
      <div className="flex items-center">
        <button
          aria-label={isDone ? '恢复为在途' : '完成'}
          title={isDone ? '点击恢复为在途' : '点击完成'}
          onClick={(e) => { e.stopPropagation(); onToggle(t) }}
          className={cn(
            'h-[18px] w-[18px] rounded-full border-[2.5px] border-foreground transition-transform hover:scale-110',
            isDone ? 'bg-[var(--neon-green)]' : 'bg-background',
            (mine === 'just-done' || mine === 'just-active') && 'ck-pop',
          )}
        />
      </div>
      <div className="min-w-0">
        <div className={cn('t-title text-sm font-bold leading-snug break-words', isDone && 'text-muted-foreground line-through')}>
          {t.title}
        </div>
        {t.remark && (
          <div
            className="mt-0.5 text-xs font-medium leading-relaxed text-muted-foreground [&_b]:font-extrabold [&_code]:border-[1.5px] [&_code]:border-foreground [&_code]:bg-[var(--neon-yellow)] [&_code]:px-1 [&_code]:font-mono [&_p]:m-0"
            dangerouslySetInnerHTML={{ __html: mdHTML(t.remark) }}
          />
        )}
      </div>
      <div className="min-w-0"><DueBadge t={t} /></div>
      <div className="hidden flex-wrap gap-1 lg:flex">
        {(t.tags ?? []).map((g) => <span key={g} className={cn(CHIP, 'bg-background')}>#{g}</span>)}
      </div>
      <div className="hidden lg:block"><SourceChip source={t.source} /></div>
      <div className={OPS}>
        <button
          aria-label="删除"
          title="删除"
          onClick={(e) => { e.stopPropagation(); onDelete(t) }}
          className="flex h-7 w-7 items-center justify-center border-2 border-transparent hover:border-foreground hover:bg-background"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-3.5 w-3.5">
            <path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" />
          </svg>
        </button>
      </div>
    </div>
  )
}

/* ================= 页面 ================= */
type View = 'active' | 'done'

export function TodosPage() {
  const [items, setItems] = useState<Todo[] | null>(null) // 当前视图列表；null=加载中
  const [counts, setCounts] = useState<TodoCounts>({ active: 0, done: 0 })
  const [view, setView] = useState<View>('active')
  const [sort, setSort] = useState<TodoSort>(TODO_SORT.created)
  const [error, setError] = useState('')
  const [anim, setAnim] = useState<Anim | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [detail, setDetail] = useState<Todo | null>(null)
  const [dlgMode, setDlgMode] = useState<'detail' | 'edit'>('detail')
  const viewRef = useRef(view); viewRef.current = view
  const sortRef = useRef(sort); sortRef.current = sort

  const load = useCallback(async (opts?: { view?: View; sort?: TodoSort | null }) => {
    const v = opts?.view ?? viewRef.current
    // sort=null ⇒ 请求不带 sort（服务端取用户偏好）；其余用传入值或当前 state
    const s = opts?.sort === null ? undefined : (opts?.sort ?? sortRef.current)
    setError('')
    try {
      const r = await todosApi.list({ status: v, sort: s, limit: 100 })
      setItems(r.todos)
      setCounts(r.counts)
      if (r.sort === TODO_SORT.created || r.sort === TODO_SORT.due) setSort(r.sort)
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '网络异常或服务不可用')
    }
  }, [])
  // 首次进页：sort 留空由服务端按用户偏好决定
  useEffect(() => { load({ sort: null }) }, [load])

  useEffect(() => {
    if (!anim) return
    const timer = setTimeout(() => setAnim(null), 950)
    return () => clearTimeout(timer)
  }, [anim])

  function switchView(v: View) {
    setView(v)
    load({ view: v })
  }

  function changeSort(s: TodoSort) {
    setSort(s)
    // 偏好按用户落库；失败不打断（下次进页回退默认）
    todosApi.setSortPref(s).catch(() => {})
    load({ sort: s })
  }

  /* ---------- 操作 ---------- */
  async function toggle(t: Todo) {
    const toDone = t.status === TODO_STATUS.active
    try {
      await todosApi.update(t.id, { status: toDone ? TODO_STATUS.done : TODO_STATUS.active })
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '操作失败')
      return
    }
    setAnim({ id: t.id, kind: toDone ? 'just-done' : 'just-active' })
    // 让动画播完再归位（行移出/移入当前列表）
    window.setTimeout(() => { load() }, toDone ? 680 : 450)
    if (toDone) {
      undoToast('已完成', {
        label: '撤销',
        onAction: async () => {
          await todosApi.update(t.id, { status: TODO_STATUS.active }).catch(() => {})
          await load()
          setAnim({ id: t.id, kind: 'just-active' })
        },
      })
    } else {
      undoToast('已恢复为在途')
    }
  }

  async function remove(t: Todo) {
    try {
      await todosApi.del(t.id)
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '删除失败')
      return
    }
    await load()
    undoToast('已删除', {
      label: '撤销',
      onAction: async () => {
        await todosApi.restore(t.id).catch(() => {})
        await load()
        setAnim({ id: t.id, kind: 'flash' })
      },
    })
  }

  async function clearDone() {
    const ids = (items ?? []).map((t) => t.id)
    if (!ids.length) return
    await Promise.all(ids.map((id) => todosApi.del(id).catch(() => {})))
    await load()
    undoToast(`已清空 ${ids.length} 条`, {
      label: '撤销',
      onAction: async () => {
        await Promise.all(ids.map((id) => todosApi.restore(id).catch(() => {})))
        await load()
      },
    })
  }

  async function onCreated(t: Todo) {
    setAddOpen(false)
    await load()
    setAnim({ id: t.id, kind: 'flash' })
    if (viewRef.current === 'done') {
      undoToast('已记入 · 当前视图不显示', { label: '切到在途', onAction: () => switchView('active') })
    } else {
      undoToast('已记入')
    }
  }

  /* ---------- 详情 / 编辑 ---------- */
  function openDetail(t: Todo) {
    setDetail(t); setDlgMode('detail')
  }

  async function saveEdit(payload: { title: string; remark: string; due: string; tags: string }) {
    if (!detail) return
    const title = payload.title.trim()
    if (!title) return
    try {
      const updated = await todosApi.update(detail.id, {
        title,
        remark: payload.remark,
        ...(payload.due ? { due_time: new Date(payload.due).toISOString() } : { clear_due: true }),
        tags: payload.tags.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
      })
      await load()
      setDetail(updated); setDlgMode('detail')
      setAnim({ id: updated.id, kind: 'flash' })
      undoToast('已保存')
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '保存失败')
    }
  }

  const loading = items === null && !error

  const head = (col3: string) => (
    <div className={cn(GRID, 'bg-foreground px-4 py-2.5 text-xs font-extrabold text-background')}>
      <div>状态</div><div>标题</div><div>{col3}</div>
      <div className="hidden lg:block">标签</div><div className="hidden lg:block">来源</div><div />
    </div>
  )

  const rows = (list: Todo[]) =>
    list.map((t) => (
      <Row key={t.id} t={t} anim={anim} onOpen={openDetail} onToggle={toggle} onDelete={remove} />
    ))

  return (
    <div className="space-y-4">
      <div className="anim-fade-up flex items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold tracking-tight">待办</h1>
          <p className="mt-0.5 text-xs font-semibold text-muted-foreground">在途 → 已完成 · AI 与 Web 同源</p>
        </div>
        <Button onClick={() => setAddOpen(true)}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.8" className="h-4 w-4">
            <path d="M12 5v14M5 12h14" />
          </svg>
          新建待办
        </Button>
      </div>

      {/* 工具行：Tab（在途/已完成）左 + 排序右 */}
      <div className="anim-fade-up flex flex-wrap items-center justify-between gap-3">
        <Tabs value={view} onValueChange={(v) => switchView(v as View)}>
          <TabsList>
            <TabsTrigger value="active">在途 <span className="text-xs opacity-60">{counts.active}</span></TabsTrigger>
            <TabsTrigger value="done">已完成 <span className="text-xs opacity-60">{counts.done}</span></TabsTrigger>
          </TabsList>
        </Tabs>
        <div className="flex items-center border-2 border-foreground bg-background shadow-[3px_3px_0px_var(--shadow-color)]" role="group" aria-label="排序方式">
          {([['created', '创建时间'], ['due', '截止时间']] as const).map(([k, label], i) => (
            <button
              key={k}
              onClick={() => changeSort(k)}
              aria-pressed={sort === k}
              className={cn('px-3.5 py-1.5 text-xs font-bold transition-colors',
                i > 0 && 'border-l-2 border-foreground',
                sort === k ? 'bg-[var(--neon-yellow)]' : 'hover:bg-primary/10')}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* 三态 */}
      {loading && (
        <div className="overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
          {head('截止')}
          {[0, 1, 2, 3, 4].map((i) => (
            <div key={i} className={cn(GRID, 'items-center border-b border-foreground/15 px-4 py-3.5 last:border-b-0')}>
              <span className="h-4 w-4 rounded-full bg-foreground/10" />
              <span className="mr-8 h-3.5 rounded bg-foreground/10" />
              <span className="h-3.5 w-24 rounded bg-foreground/10" />
              <span className="hidden h-3.5 w-14 rounded bg-foreground/10 lg:block" />
              <span className="hidden h-3.5 w-12 rounded bg-foreground/10 lg:block" />
              <span />
            </div>
          ))}
        </div>
      )}

      {error && (
        <div className="anim-pop border-3 border-foreground border-l-8 border-l-destructive bg-card px-6 py-7 text-center shadow-[4px_4px_0px_var(--shadow-color)]">
          <div className="text-base font-extrabold">加载失败</div>
          <p className="mt-1 text-sm font-semibold text-muted-foreground">{error}</p>
          <Button className="mt-4" onClick={() => { setItems(null); load() }}>重试</Button>
        </div>
      )}

      {!loading && !error && view === 'done' && (
        <>
          <div className="flex items-center justify-between text-xs font-bold">
            <span className="text-muted-foreground">已完成 {(items ?? []).length} 条</span>
            {(items ?? []).length > 0 && <Button size="sm" variant="outline" onClick={clearDone}>清空已完成</Button>}
          </div>
          {(items ?? []).length > 0 ? (
            <div className="anim-fade-up overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
              {head('完成')}
              {rows(items ?? [])}
            </div>
          ) : (
            <EmptyState title="还没有完成的待办" />
          )}
        </>
      )}

      {!loading && !error && view === 'active' && (
        (items ?? []).length > 0 ? (
          <div className="anim-fade-up overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
            {head('截止')}
            {rows(items ?? [])}
          </div>
        ) : (
          <EmptyState title="没有在途的待办" hint />
        )
      )}

      <AddDialog open={addOpen} onClose={() => setAddOpen(false)} onCreated={onCreated} />
      <TodoDialog
        item={detail}
        mode={dlgMode}
        onModeChange={setDlgMode}
        onClose={() => { setDetail(null); setDlgMode('detail') }}
        onSave={saveEdit}
      />
    </div>
  )
}

function EmptyState({ title, hint }: { title: string; hint?: boolean }) {
  return (
    <div className="anim-fade-up border-3 border-dashed border-foreground bg-card/60 px-6 py-10 text-center">
      <div className="text-base font-extrabold">{title}</div>
      {hint && (
        <>
          <p className="mt-1.5 text-sm font-semibold text-muted-foreground">点右上角「新建待办」，或让 AI 帮你记：</p>
          <div className="mt-3 inline-block border-2 border-foreground bg-background px-3 py-2 text-left font-mono text-xs font-semibold shadow-[3px_3px_0px_var(--shadow-color)]">
            extbrain todo add "买牛奶" --due "明天 10:00"
          </div>
        </>
      )}
    </div>
  )
}

/* ================= 新建待办（表单弹窗，与编辑弹窗同构） ================= */
function AddDialog({ open, onClose, onCreated }: {
  open: boolean
  onClose: () => void
  onCreated: (t: Todo) => void
}) {
  const [title, setTitle] = useState('')
  const [remark, setRemark] = useState('')
  const [due, setDue] = useState('')
  const [tags, setTags] = useState('')
  const [titleErr, setTitleErr] = useState(false)

  useEffect(() => {
    if (open) { setTitle(''); setRemark(''); setDue(''); setTags(''); setTitleErr(false) }
  }, [open])

  async function create() {
    const t = title.trim()
    if (!t) { setTitleErr(true); return }
    try {
      const created = await todosApi.create({
        title: t,
        remark,
        ...(due ? { due_time: new Date(due).toISOString() } : {}),
        tags: tags.split(/[,，]/).map((s) => s.trim()).filter(Boolean),
      })
      onCreated(created)
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '创建失败')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-[460px]">
        <DialogHeader>
          <DialogTitle>新建待办</DialogTitle>
        </DialogHeader>
        <div
          className="space-y-4"
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
              e.preventDefault()
              create()
            }
          }}
        >
          <div className="space-y-2">
            <Label>标题</Label>
            <Input
              autoFocus
              value={title}
              placeholder="要做什么？"
              aria-invalid={titleErr}
              onChange={(e) => { setTitle(e.target.value); setTitleErr(false) }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') { e.preventDefault(); create() }
              }}
            />
            {titleErr && (
              <div className="flex items-center gap-1.5 text-xs font-bold text-destructive">
                <span className="h-2 w-2 rounded-full border-2 border-foreground bg-destructive" />
                标题必填
              </div>
            )}
          </div>
          <div className="space-y-2">
            <Label>备注（支持 Markdown）</Label>
            <Textarea rows={3} value={remark} onChange={(e) => setRemark(e.target.value)} placeholder="可选；**加粗**、`代码`…" className="resize-none" />
          </div>
          <div className="flex gap-3">
            <div className="flex-1 space-y-2">
              <Label>截止时间</Label>
              <input
                type="datetime-local"
                value={due}
                onChange={(e) => setDue(e.target.value)}
                className="h-11 w-full border-3 border-foreground bg-background px-3 text-sm font-bold shadow-[4px_4px_0px_var(--shadow-color)] outline-none bk-interactive focus-visible:shadow-[6px_6px_0px_var(--shadow-color)]"
              />
            </div>
            <div className="flex-1 space-y-2">
              <Label>标签（逗号分隔）</Label>
              <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="工作, dev" />
            </div>
          </div>
        </div>
        <DialogFooter className="items-center gap-2 sm:justify-end">
          <span className="mr-auto hidden text-xs font-semibold text-muted-foreground sm:block">
            <kbd className="border-2 border-foreground bg-background px-1 font-mono">↵</kbd> 创建 ·{' '}
            <kbd className="border-2 border-foreground bg-background px-1 font-mono">Esc</kbd> 关闭
          </span>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={create}>创建</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ================= 详情 / 编辑弹窗（右上角关闭、右下角编辑） ================= */
function TodoDialog({ item, mode, onModeChange, onClose, onSave }: {
  item: Todo | null
  mode: 'detail' | 'edit'
  onModeChange: (m: 'detail' | 'edit') => void
  onClose: () => void
  onSave: (p: { title: string; remark: string; due: string; tags: string }) => void
}) {
  const [title, setTitle] = useState('')
  const [remark, setRemark] = useState('')
  const [due, setDue] = useState('')
  const [tags, setTags] = useState('')

  // 进入编辑态时从当前条目初始化表单
  useEffect(() => {
    if (item && mode === 'edit') {
      setTitle(item.title)
      setRemark(item.remark ?? '')
      setDue(item.due_time ? toLocalInput(new Date(item.due_time)) : '')
      setTags((item.tags ?? []).join(', '))
    }
  }, [item, mode])

  if (!item) return null
  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-[460px]">
        <DialogHeader>
          <DialogTitle>{mode === 'detail' ? '待办详情' : '编辑待办'}</DialogTitle>
        </DialogHeader>

        {mode === 'detail' ? (
          <>
            <div className="space-y-3">
              <div className="text-base font-extrabold leading-snug break-words">{item.title}</div>
              <div className="flex flex-wrap gap-1.5">
                <StatusChip status={item.status} />
                {(item.status === TODO_STATUS.done ? item.completed_at : item.due_time) && <DueBadge t={item} />}
                {(item.tags ?? []).map((g) => <span key={g} className={cn(CHIP, 'bg-background')}>#{g}</span>)}
                <SourceChip source={item.source} />
              </div>
              {item.remark && (
                <div
                  className="border-t-2 border-foreground/20 pt-3 text-sm leading-relaxed [&_b]:font-extrabold [&_code]:border-[1.5px] [&_code]:border-foreground [&_code]:bg-[var(--neon-yellow)] [&_code]:px-1 [&_code]:font-mono"
                  dangerouslySetInnerHTML={{ __html: mdHTML(item.remark) }}
                />
              )}
              <div className="border-t border-foreground/15 pt-2 text-xs font-semibold text-muted-foreground">
                创建于 {fmtFull(new Date(item.create_time))}
                {item.status === TODO_STATUS.done && item.completed_at && <> · 完成于 {fmtFull(new Date(item.completed_at))}</>}
              </div>
            </div>
            <DialogFooter>
              <Button onClick={() => onModeChange('edit')}>编辑</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <div
              className="space-y-4"
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                  e.preventDefault()
                  onSave({ title, remark, due, tags })
                }
              }}
            >
              <div className="space-y-2">
                <Label>标题</Label>
                <Input value={title} onChange={(e) => setTitle(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>备注（支持 Markdown）</Label>
                <Textarea rows={3} value={remark} onChange={(e) => setRemark(e.target.value)} placeholder="可选；**加粗**、`代码`…" className="resize-none" />
              </div>
              <div className="flex gap-3">
                <div className="flex-1 space-y-2">
                  <Label>截止时间</Label>
                  <input
                    type="datetime-local"
                    value={due}
                    onChange={(e) => setDue(e.target.value)}
                    className="h-11 w-full border-3 border-foreground bg-background px-3 text-sm font-bold shadow-[4px_4px_0px_var(--shadow-color)] outline-none bk-interactive focus-visible:shadow-[6px_6px_0px_var(--shadow-color)]"
                  />
                </div>
                <div className="flex-1 space-y-2">
                  <Label>标签（逗号分隔）</Label>
                  <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="工作, dev" />
                </div>
              </div>
            </div>
            <DialogFooter className="items-center gap-2 sm:justify-end">
              <span className="mr-auto hidden text-xs font-semibold text-muted-foreground sm:block">
                <kbd className="border-2 border-foreground bg-background px-1 font-mono">⌘</kbd>{' '}
                <kbd className="border-2 border-foreground bg-background px-1 font-mono">↵</kbd> 保存
              </span>
              <Button variant="outline" onClick={() => onModeChange('detail')}>取消</Button>
              <Button onClick={() => onSave({ title, remark, due, tags })}>保存</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
