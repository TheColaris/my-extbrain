import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { ApiError, notePermApi, notesApi, reposApi, type NoteMeta, type NotePermRule, type RepoItem } from '@/lib/api'
import { NotePermDialog } from '@/components/note-perm-dialog'
import { cn } from '@/lib/utils'

/* ================= 工具 ================= */
const baseName = (p: string) => p.split('/').pop() ?? p
const dirOf = (p: string) => {
  const i = p.lastIndexOf('/')
  return i < 0 ? '' : p.slice(0, i)
}
/** 显示层规则：title 等于文件名（后端缺省值，如 "x.md"）时去扩展名 */
function dispTitle(n: { path: string; title: string }) {
  const b = baseName(n.path)
  const t = n.title || b
  return t === b ? b.replace(/\.md$/i, '') : t
}
const pad = (n: number) => String(n).padStart(2, '0')
function fmtRel(iso: string): string {
  const d = new Date(iso)
  const now = new Date()
  const diff = +now - +d
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (diff < 60e3) return '刚刚'
  if (diff < 3600e3) return `${Math.floor(diff / 60e3)} 分钟前`
  const sameDay = (a: Date, b: Date) =>
    a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
  if (sameDay(d, now)) return `今天 ${hm}`
  const yest = new Date(); yest.setDate(yest.getDate() - 1)
  if (sameDay(d, yest)) return `昨天 ${hm}`
  if (d.getFullYear() === now.getFullYear()) return `${d.getMonth() + 1} 月 ${d.getDate()} 日`
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
const parseTags = (s: string) => s.split(/[,，]/).map((x) => x.trim()).filter(Boolean)
/** 规范化新路径：去首尾 /；末段无扩展名自动补 .md（目录由 / 自动生成） */
function normPath(p: string): string {
  const segs = p.trim().replace(/^\/+/, '').replace(/\/+$/, '').split('/').filter(Boolean)
  const last = segs[segs.length - 1] ?? ''
  if (last && !last.includes('.')) segs[segs.length - 1] = `${last}.md`
  return segs.join('/')
}
function pathError(p: string): string {
  if (!p) return '路径不能为空'
  if (p.includes('//') || p.split('/').some((s) => s === '..' || s === '.')) return '路径含非法段（// 、. 或 ..）'
  if (p.length > 500) return '路径过长（≤500）'
  return ''
}
/** 摘录去 MD 记号（搜索高亮前） */
const stripMD = (s: string) => s.replace(/[*`#>]/g, '')

/* ================= 目录树 ================= */
interface DirNode { name: string; path: string; count: number; children: DirNode[] }

function buildTree(notes: NoteMeta[]): DirNode {
  const root: DirNode = { name: '', path: '', count: 0, children: [] }
  for (const n of notes) {
    const segs = n.path.split('/')
    segs.pop()
    let cur = root
    let acc = ''
    for (const s of segs) {
      acc = acc ? `${acc}/${s}` : s
      let child = cur.children.find((c) => c.name === s)
      if (!child) {
        child = { name: s, path: acc, count: 0, children: [] }
        cur.children.push(child)
      }
      child.count++
      cur = child
    }
  }
  root.count = notes.length
  const sortRec = (node: DirNode) => {
    node.children.sort((a, b) => a.name.localeCompare(b.name, 'zh'))
    node.children.forEach(sortRec)
  }
  sortRec(root)
  return root
}

const ICON = {
  caret: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" className="h-[11px] w-[11px] transition-transform"><path d="m9 6 6 6-6 6" /></svg>,
  folder: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-[15px] w-[15px]"><path d="M3 7a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /></svg>,
  clock: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-[15px] w-[15px]"><circle cx="12" cy="12" r="8.5" /><path d="M12 7.5V12l3 2" /></svg>,
  books: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-[15px] w-[15px]"><path d="M4 4h7v16H4z" /><path d="M11 4h9v16h-9" /><path d="M14 9h3M14 13h3" /></svg>,
  open: <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" className="h-[11px] w-[11px]"><path d="M7 17 17 7" /><path d="M9 7h8v8" /></svg>,
}

/* 选中目标：全部 / 最近更新 / 某目录 */
type Sel = { kind: 'all' } | { kind: 'recent' } | { kind: 'dir'; path: string }
const selDir = (s: Sel) => (s.kind === 'dir' ? s.path : '')

/* ================= 目录树节点 ================= */
const LOCK_ICON = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className="h-3 w-3">
    <rect x="4" y="10" width="16" height="10" rx="2" />
    <path d="M8 10V7a4 4 0 0 1 8 0v3" />
  </svg>
)
const PERM_ICON = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className="h-3 w-3">
    <rect x="4" y="10" width="16" height="10" rx="2" />
    <path d="M8 10V7a4 4 0 0 1 8 0v3" />
  </svg>
)

function TreeNode({ node, depth, sel, expanded, onToggle, onSelect, locked, onPerm }: {
  node: DirNode
  depth: number
  sel: Sel
  expanded: Set<string>
  onToggle: (p: string) => void
  onSelect: (p: string) => void
  locked: boolean
  onPerm: (path: string) => void
}) {
  const hasKids = node.children.length > 0
  const open = expanded.has(node.path)
  const active = sel.kind === 'dir' && sel.path === node.path
  return (
    <>
      <div
        className={cn(
          'group/row mb-px flex h-[30px] cursor-pointer items-center gap-1.5 rounded-lg border-2 px-1.5 text-[12.5px] font-semibold',
          active ? 'border-foreground bg-[var(--neon-blue)] shadow-[2px_2px_0px_var(--shadow-color)]' : 'border-transparent hover:border-foreground hover:bg-background',
        )}
        style={{ paddingLeft: 6 + depth * 14 }}
        data-dir={node.path}
        onClick={() => onSelect(node.path)}
      >
        <button
          type="button"
          aria-label={open ? '折叠' : '展开'}
          className={cn('flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground', !hasKids && 'invisible', open && '[&>svg]:rotate-90')}
          onClick={(e) => { e.stopPropagation(); onToggle(node.path) }}
        >
          {ICON.caret}
        </button>
        <span className="shrink-0">{ICON.folder}</span>
        <span className="min-w-0 flex-1 truncate">{node.name}</span>
        {locked && <span className="shrink-0 text-muted-foreground" data-role="dir-lock">{LOCK_ICON}</span>}
        <button
          type="button"
          aria-label={`目录权限 ${node.path}`}
          data-role="perm-btn"
          title="目录权限"
          className="hidden h-[22px] w-[22px] shrink-0 items-center justify-center rounded-md border-2 border-foreground bg-[var(--neon-yellow)] text-foreground group-hover/row:flex hover:shadow-[2px_2px_0px_var(--shadow-color)]"
          onClick={(e) => { e.stopPropagation(); onPerm(node.path) }}
        >
          {PERM_ICON}
        </button>
        <span className={cn('shrink-0 border-[1.5px] border-foreground px-1 text-[10.5px] font-bold', active ? 'bg-background' : 'bg-background/70')}>{node.count}</span>
      </div>
      {hasKids && open && node.children.map((c) => (
        <TreeNode key={c.path} node={c} depth={depth + 1} sel={sel} expanded={expanded} onToggle={onToggle} onSelect={onSelect} locked={locked} onPerm={onPerm} />
      ))}
    </>
  )
}

/* ================= 页面 ================= */
export function NotesPage() {
  const [sp, setSp] = useSearchParams()
  const openPath = sp.get('note') // 当前浮窗打开的笔记（高亮行用；点行 = 打开浮窗）
  const [notes, setNotes] = useState<NoteMeta[] | null>(null) // null = 加载中
  const [error, setError] = useState('')
  const [sel, setSel] = useState<Sel>({ kind: 'all' })
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [sort, setSort] = useState<'updated' | 'name'>('updated')
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<{ repo_name?: string; path: string; title: string; snippet: string }[] | null>(null)
  const [treeDrawer, setTreeDrawer] = useState(false)
  const [dlgNew, setDlgNew] = useState(false)
  const [permRules, setPermRules] = useState<NotePermRule[]>([])
  // 仓库（迁移 0012）：null=加载中；curRepoId 持久化到 localStorage
  const [repos, setRepos] = useState<RepoItem[] | null>(null)
  const [curRepoId, setCurRepoId] = useState<number | null>(null)
  const [repoMenu, setRepoMenu] = useState(false)
  const [mgmtOpen, setMgmtOpen] = useState(false)
  const [repoForm, setRepoForm] = useState<{ mode: 'new' } | { mode: 'rename'; repo: RepoItem } | null>(null)
  const [permDlg, setPermDlg] = useState<{ folder: string } | null>(null) // null=关；folder=''=仓库级规则

  const searchSeq = useRef(0)

  const curRepo = useMemo(
    () => repos?.find((r) => r.id === curRepoId) ?? null,
    [repos, curRepoId],
  )

  /** 点笔记行 → 弹浮窗（push：浏览器返回键关闭；repo=所属仓库，浮窗按仓库寻址） */
  const openNote = (p: string, repoName?: string) => {
    const r = repoName ?? curRepo?.name ?? ''
    setSp(r ? { note: p, repo: r } : { note: p })
  }

  /* ---------- 加载 ---------- */
  const load = useCallback(async () => {
    if (curRepoId == null) return []
    try {
      const r = await notesApi.list('', curRepo?.name)
      setNotes(r.notes ?? [])
      setError('')
      // 默认展开一级目录（首次加载时）
      setExpanded((prev) => {
        if (prev.size) return prev
        const first = new Set<string>()
        for (const n of r.notes ?? []) {
          const d = dirOf(n.path)
          if (d) first.add(d.split('/')[0])
        }
        return first
      })
      return r.notes ?? []
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '网络异常或服务不可用')
      setNotes([])
      return []
    }
  }, [curRepoId, curRepo?.name])

  /** 仓库清单加载；首次选定默认（或 localStorage 记忆） */
  const loadRepos = useCallback(async (): Promise<RepoItem[]> => {
    try {
      const r = await reposApi.list()
      const list = r.repos ?? []
      setRepos(list)
      setCurRepoId((prev) => {
        if (prev != null && list.some((x) => x.id === prev)) return prev
        const stored = Number(localStorage.getItem('extbrain:repo') ?? '')
        if (list.some((x) => x.id === stored)) return stored
        return list.find((x) => x.is_default === 1)?.id ?? list[0]?.id ?? null
      })
      return list
    } catch {
      setRepos([])
      return []
    }
  }, [])

  const switchRepo = (id: number) => {
    if (id === curRepoId) return
    localStorage.setItem('extbrain:repo', String(id))
    setCurRepoId(id)
    setSel({ kind: 'all' })
    setExpanded(new Set())
    setQ('')
    setRepoMenu(false)
  }

  // 首次加载 + 浮窗内 删除/恢复/重命名/保存 后联动刷新列表
  useEffect(() => {
    void loadRepos()
    notePermApi.list().then((r) => setPermRules(r.rules ?? [])).catch(() => setPermRules([]))
    const h = () => { void load() }
    window.addEventListener('extbrain:notes-changed', h)
    return () => window.removeEventListener('extbrain:notes-changed', h)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  /* ---------- 搜索（防抖 300ms，seq 防乱序） ---------- */
  useEffect(() => {
    const query = q.trim()
    if (!query) {
      setHits(null)
      return
    }
    const seq = ++searchSeq.current
    const timer = setTimeout(async () => {
      try {
        const r = await notesApi.search(query)
        if (seq === searchSeq.current) setHits(r.hits ?? [])
      } catch {
        if (seq === searchSeq.current) setHits([])
      }
    }, 300)
    return () => clearTimeout(timer)
  }, [q])

  /* ---------- 树数据 ---------- */
  const tree = useMemo(() => buildTree(notes ?? []), [notes])
  const findDir = useCallback((root: DirNode, path: string): DirNode | null => {
    if (!path) return root
    const segs = path.split('/')
    let cur: DirNode | undefined = root
    for (const s of segs) {
      cur = cur?.children.find((c) => c.name === s)
      if (!cur) return null
    }
    return cur ?? null
  }, [])

  const toggleExpand = (p: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p)
      else next.add(p)
      return next
    })
  }

  const gotoDir = (path: string) => {
    setQ('')
    setSel({ kind: 'dir', path })
    setTreeDrawer(false)
    if (path) {
      setExpanded((prev) => {
        const next = new Set(prev)
        const segs = path.split('/')
        let acc = ''
        for (const s of segs) {
          acc = acc ? `${acc}/${s}` : s
          next.add(acc)
        }
        return next
      })
    }
  }

  /* ---------- 列表数据 ---------- */
  const sortList = useCallback((list: NoteMeta[]) => {
    const arr = list.slice()
    if (sort === 'name') arr.sort((a, b) => a.path.localeCompare(b.path, 'zh'))
    else arr.sort((a, b) => +new Date(b.update_time) - +new Date(a.update_time))
    return arr
  }, [sort])

  const dirPath = selDir(sel)
  const dirNode = useMemo(() => findDir(tree, dirPath), [tree, dirPath, findDir])
  const childDirs = dirNode?.children ?? []
  const dirNotes = useMemo(
    () => sortList((notes ?? []).filter((n) => dirOf(n.path) === dirPath)),
    [notes, dirPath, sortList],
  )
  const recentNotes = useMemo(
    () => sortList(notes ?? []),
    [notes, sortList],
  )

  /* ---------- 操作 ---------- */
  async function doCreate(input: { path: string; title: string; tags: string; content: string }) {
    const path = normPath(input.path)
    const title = input.title.trim() || baseName(path)
    const content = input.content.trim() || `# ${title.replace(/\.md$/i, '')}\n`
    await notesApi.put(path, { content, title, tags: parseTags(input.tags) }, curRepo?.name)
    await load()
    gotoDir(dirOf(path))
    openNote(path)
    toast('已创建')
  }

  /* ---------- 渲染 ---------- */
  const loading = notes === null && !error
  const isEmptyKB = !loading && !error && (notes ?? []).length === 0
  const searching = q.trim().length > 0

  /* ---------- 仓库切换器（树顶；仓库 > 文件夹 > 笔记） ---------- */
  const repoSwitcher = (
    <div className="relative border-b-3 border-foreground" data-role="repo-switcher">
      <button
        type="button"
        data-role="repo-btn"
        className="flex w-full items-center gap-1.5 bg-[var(--neon-yellow)] px-2.5 py-2 text-left text-[13px] font-extrabold"
        onClick={() => setRepoMenu((v) => !v)}
      >
        <span className="min-w-0 flex-1 truncate">{curRepo?.name ?? '…'}</span>
        {curRepo?.is_default === 1 && (
          <span className="shrink-0 rounded border-2 border-foreground bg-[var(--neon-lime)] px-1 text-[10px] font-bold" data-role="repo-default">默认</span>
        )}
        {permRules.some((r) => r.repo_id === curRepoId && r.folder_path === '' && r.mode === 'allow') && (
          <span className="shrink-0 text-muted-foreground" data-role="repo-lock">{LOCK_ICON}</span>
        )}
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className={cn('h-3.5 w-3.5 shrink-0 transition-transform', repoMenu && 'rotate-180')}><path d="m6 9 6 6 6-6" /></svg>
      </button>
      {repoMenu && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setRepoMenu(false)} />
          <div className="absolute inset-x-2 top-full z-50 mt-1 overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[5px_5px_0px_var(--shadow-color)]" data-role="repo-menu">
            {(repos ?? []).map((r) => (
              <button
                key={r.id}
                type="button"
                data-role="repo-item"
                className={cn('flex w-full items-center gap-2 px-3 py-2 text-left text-[12.5px] font-bold hover:bg-[var(--neon-yellow)]/30', r.id === curRepoId && 'bg-[var(--neon-yellow)]/60')}
                onClick={() => switchRepo(r.id)}
              >
                <span className="min-w-0 flex-1 truncate">{r.name}</span>
                {r.is_default === 1 && <span className="shrink-0 rounded border-2 border-foreground bg-[var(--neon-lime)] px-1 text-[10px] font-bold">默认</span>}
                {permRules.some((x) => x.repo_id === r.id && x.folder_path === '' && x.mode === 'allow') && <span className="shrink-0 text-muted-foreground">{LOCK_ICON}</span>}
                <span className="shrink-0 border-[1.5px] border-foreground bg-background px-1 text-[10.5px] font-bold">{r.note_count}</span>
              </button>
            ))}
            <div className="flex border-t-2 border-foreground">
              <button type="button" className="flex-1 px-2 py-2 text-[12px] font-bold hover:bg-[var(--neon-yellow)]" data-role="repo-manage" onClick={() => { setRepoMenu(false); setMgmtOpen(true) }}>
                管理仓库
              </button>
              <button type="button" className="flex-1 border-l-2 border-foreground px-2 py-2 text-[12px] font-bold hover:bg-[var(--neon-yellow)]" data-role="repo-new" onClick={() => { setRepoMenu(false); setRepoForm({ mode: 'new' }) }}>
                ＋ 新建仓库
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )

  const treePane = (
    <div className="flex h-full min-h-0 flex-col">
      {repoSwitcher}
      <div className="flex gap-1.5 border-b-3 border-foreground p-2.5">
        <Input
          className="h-8 text-[12.5px]"
          data-role="search"
          placeholder="搜索全部笔记…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {loading ? (
          [0, 1, 2, 3, 4].map((i) => (
            <div key={i} className="px-2 py-2"><span className="block h-3 w-1/2 rounded bg-foreground/10" /></div>
          ))
        ) : error ? (
          <p className="px-2.5 py-3 text-xs font-bold text-muted-foreground">目录加载失败</p>
        ) : (
          <>
            <div
              className={cn(
                'mb-px flex h-[30px] cursor-pointer items-center gap-1.5 rounded-lg border-2 px-1.5 text-[12.5px] font-semibold',
                sel.kind === 'recent' ? 'border-foreground bg-[var(--neon-yellow)] shadow-[2px_2px_0px_var(--shadow-color)]' : 'border-transparent hover:border-foreground hover:bg-background',
              )}
              onClick={() => { setQ(''); setSel({ kind: 'recent' }); setTreeDrawer(false) }}
            >
              <span className="w-4 shrink-0" />
              <span className="shrink-0">{ICON.clock}</span>
              <span className="min-w-0 flex-1 truncate">最近更新</span>
              <span className="shrink-0 border-[1.5px] border-foreground bg-background px-1 text-[10.5px] font-bold">{notes?.length ?? 0}</span>
            </div>
            <div
              className={cn(
                'mb-px flex h-[30px] cursor-pointer items-center gap-1.5 rounded-lg border-2 px-1.5 text-[12.5px] font-semibold',
                sel.kind === 'all' ? 'border-foreground bg-[var(--neon-yellow)] shadow-[2px_2px_0px_var(--shadow-color)]' : 'border-transparent hover:border-foreground hover:bg-background',
              )}
              onClick={() => { setQ(''); setSel({ kind: 'all' }); setTreeDrawer(false) }}
            >
              <span className="w-4 shrink-0" />
              <span className="shrink-0">{ICON.books}</span>
              <span className="min-w-0 flex-1 truncate">全部笔记</span>
              {permRules.some((r) => r.repo_id === curRepoId && r.folder_path === '' && r.mode === 'allow') && (
                <span className="shrink-0 text-muted-foreground" data-role="dir-lock">{LOCK_ICON}</span>
              )}
              <button
                type="button"
                aria-label="仓库权限"
                data-role="perm-btn-root"
                title="仓库权限（本仓库默认可见性）"
                className="hidden h-[22px] w-[22px] shrink-0 items-center justify-center rounded-md border-2 border-foreground bg-[var(--neon-yellow)] text-foreground hover:shadow-[2px_2px_0px_var(--shadow-color)] sm:flex"
                onClick={(e) => { e.stopPropagation(); setPermDlg({ folder: '' }) }}
              >
                {PERM_ICON}
              </button>
              <span className="shrink-0 border-[1.5px] border-foreground bg-background px-1 text-[10.5px] font-bold">{notes?.length ?? 0}</span>
            </div>
            {tree.children.length === 0 && (
              <div className="px-2.5 py-6 text-center text-[11.5px] font-semibold leading-relaxed text-muted-foreground">
                仓库「{curRepo?.name ?? ''}」还是空的<br />新建第一篇笔记，或从别处移动过来
              </div>
            )}
            {tree.children.map((c) => (
              <TreeNode key={c.path} node={c} depth={1} sel={sel} expanded={expanded} onToggle={toggleExpand} onSelect={gotoDir}
                locked={permRules.some((r) => r.repo_id === curRepoId && r.folder_path === c.path && r.mode === 'allow')}
                onPerm={(fp) => setPermDlg({ folder: fp })} />
            ))}
          </>
        )}
      </div>
      <div className="border-t border-foreground/15 px-3 py-2 text-[11px] font-semibold text-muted-foreground">目录由笔记路径自动生成 · 路径为当前仓库内相对路径</div>
    </div>
  )

  /* 面包屑 */
  const crumbParts: { label: string; sel: Sel | null }[] = [{ label: '全部', sel: { kind: 'all' } }]
  if (dirPath) {
    const segs = dirPath.split('/')
    let acc = ''
    for (const s of segs) {
      acc = acc ? `${acc}/${s}` : s
      crumbParts.push({ label: s, sel: { kind: 'dir', path: acc } })
    }
  }

  const listHead = (
    <div className="border-b-3 border-foreground p-2.5">
      {searching ? (
        <>
          <div className="flex items-center gap-1 text-sm font-extrabold">
            <span className="min-w-0 truncate">搜索「{q.trim()}」</span>
          </div>
          <div className="mt-2 flex items-center justify-between gap-2">
            <span className="text-[11.5px] font-semibold text-muted-foreground">{hits ? `${hits.length} 条命中` : '搜索中…'}</span>
            <Button size="sm" variant="outline" onClick={() => setQ('')}>清除搜索</Button>
          </div>
        </>
      ) : sel.kind === 'recent' ? (
        <>
          <div className="text-sm font-extrabold">最近更新</div>
          <div className="mt-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-1.5">
            <span className="whitespace-nowrap text-[11.5px] font-semibold text-muted-foreground">{recentNotes.length} 篇 · 按更新时间倒序</span>
            <Button size="sm" data-role="new" onClick={() => setDlgNew(true)}>＋ 新建</Button>
          </div>
        </>
      ) : (
        <>
          <div className="flex min-w-0 items-center gap-1 text-sm font-extrabold">
            {crumbParts.map((p, i) => (
              <span key={i} className="flex min-w-0 items-center gap-1">
                {p.sel ? (
                  <button
                    type="button"
                    className="rounded px-0.5 hover:bg-[var(--neon-yellow)]"
                    onClick={() => gotoDir(p.sel!.kind === 'dir' ? p.sel!.path : '')}
                  >
                    {p.label}
                  </button>
                ) : (
                  <span className="truncate">{p.label}</span>
                )}
                {i < crumbParts.length - 1 && <span className="text-muted-foreground">/</span>}
              </span>
            ))}
          </div>
          <div className="mt-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-1.5">
            <span className="whitespace-nowrap text-[11.5px] font-semibold text-muted-foreground">
              {dirNotes.length} 篇笔记{childDirs.length > 0 && ` · ${childDirs.length} 个目录`}
            </span>
            <span className="flex shrink-0 gap-1.5">
              <Button size="sm" variant="outline" onClick={() => setSort(sort === 'updated' ? 'name' : 'updated')}>
                ⇅ {sort === 'updated' ? '最近更新' : '名称'}
              </Button>
              <Button size="sm" data-role="new" onClick={() => setDlgNew(true)}>＋ 新建</Button>
            </span>
          </div>
        </>
      )}
    </div>
  )

  const noteRow = (n: NoteMeta, showPath: boolean) => (
    <div
      key={n.path}
      className={cn(
        'group relative cursor-pointer border-b border-foreground/15 px-3 py-2 transition-colors last:border-b-0',
        openPath === n.path ? 'bg-[var(--neon-yellow)]' : 'hover:bg-[var(--neon-yellow)]/10',
      )}
      data-note={n.path}
      onClick={() => { openNote(n.path); setTreeDrawer(false) }}
    >
      {openPath === n.path && <span className="absolute inset-y-0 left-0 w-1 bg-foreground" />}
      <div className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate text-[13.5px] font-extrabold leading-snug">{dispTitle(n)}</span>
        {(n.tags ?? []).slice(0, 2).map((t) => (
          <span key={t} className="inline-flex shrink-0 items-center border-2 border-foreground bg-background px-1 text-[10.5px] font-bold">#{t}</span>
        ))}
        <span className="shrink-0 text-[11px] font-bold text-muted-foreground">{fmtRel(n.update_time)}</span>
      </div>
      <div className="mt-0.5 flex items-center gap-2 text-[11.5px] font-semibold text-muted-foreground">
        <span className="min-w-0 flex-1 truncate font-mono">{showPath ? n.path : baseName(n.path)}</span>
        <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center border-2 border-foreground bg-background font-extrabold shadow-[1.5px_1.5px_0px_var(--shadow-color)] opacity-0 transition-opacity duration-150 group-hover:opacity-100" title="打开（浮窗）">
          {ICON.open}
        </span>
      </div>
    </div>
  )

  const hl = (s: string, query: string) => {
    const qq = query.trim()
    if (!qq) return s
    const idx = s.toLowerCase().indexOf(qq.toLowerCase())
    if (idx < 0) return s
    return (
      <>
        {s.slice(0, idx)}
        <mark className="bg-[var(--neon-yellow)] px-px font-extrabold">{s.slice(idx, idx + qq.length)}</mark>
        {s.slice(idx + qq.length)}
      </>
    )
  }

  const listBody = (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {loading ? (
        [0, 1, 2, 3, 4].map((i) => (
          <div key={i} className="border-b border-foreground/15 px-3 py-3.5">
            <span className="block h-3.5 w-3/5 rounded bg-foreground/10" />
            <span className="mt-2 block h-3 w-1/3 rounded bg-foreground/10" />
          </div>
        ))
      ) : error ? (
        <div className="anim-pop m-3.5 border-3 border-foreground border-l-8 border-l-destructive bg-card px-4 py-5 text-center">
          <div className="text-sm font-extrabold">加载失败</div>
          <p className="mt-1 text-xs font-semibold text-muted-foreground">{error}</p>
          <Button size="sm" className="mt-3" onClick={() => { setNotes(null); load() }}>重试</Button>
        </div>
      ) : searching ? (
        (hits ?? []).length > 0 ? (
          (hits ?? []).map((h) => (
            <div
              key={`${h.repo_name ?? ''}/${h.path}`}
              className={cn('cursor-pointer border-b border-foreground/15 px-3 py-2.5 hover:bg-[var(--neon-yellow)]/10', openPath === h.path && 'bg-[var(--neon-yellow)]')}
              onClick={() => openNote(h.path, h.repo_name)}
            >
              <div className="text-[13.5px] font-extrabold leading-snug">
                {h.repo_name && h.repo_name !== curRepo?.name && (
                  <span className="mr-1.5 inline-block translate-y-[-1px] rounded border-2 border-foreground bg-[var(--neon-blue)] px-1 text-[10px] font-bold" data-role="hit-repo">{h.repo_name}</span>
                )}
                {hl(dispTitle(h), q)}
              </div>
              <div className="mt-0.5 truncate font-mono text-[11.5px] font-semibold text-muted-foreground">{hl(h.path, q)}</div>
              <div className="mt-1 text-[11.5px] font-medium leading-relaxed text-muted-foreground">
                …{hl(stripMD(h.snippet), q)}…
              </div>
            </div>
          ))
        ) : (
          <div className="m-3.5 border-3 border-dashed border-foreground bg-card/60 px-5 py-8 text-center">
            <div className="text-sm font-extrabold">没有命中</div>
            <p className="mt-1.5 text-xs font-semibold text-muted-foreground">换个词试试，或清除搜索回到浏览</p>
            <Button size="sm" className="mt-3" onClick={() => setQ('')}>清除搜索</Button>
          </div>
        )
      ) : isEmptyKB ? (
        <div className="m-3.5 border-3 border-dashed border-foreground bg-card/60 px-5 py-8 text-center">
          <div className="text-sm font-extrabold">仓库「{curRepo?.name ?? ''}」还是空的</div>
          <p className="mt-1.5 text-xs font-semibold text-muted-foreground">点「新建」手写一篇；或让 AI 通过 CLI 写进来：</p>
          <div className="mt-3 inline-block border-2 border-foreground bg-background px-2.5 py-1.5 text-left font-mono text-[11px] font-semibold shadow-[3px_3px_0px_var(--shadow-color)]">
            extbrain note push ./草稿.md --path ai/入门.md
          </div>
        </div>
      ) : sel.kind === 'recent' ? (
        recentNotes.length ? recentNotes.map((n) => noteRow(n, true)) : null
      ) : (
        <>
          {childDirs.map((d) => (
            <div
              key={d.path}
              className="flex cursor-pointer items-center gap-2 border-b border-foreground/15 px-3 py-2.5 text-[13px] font-bold hover:bg-[var(--neon-blue)]/20"
              data-dir={d.path}
              onClick={() => gotoDir(d.path)}
            >
              <span className="shrink-0">{ICON.folder}</span>
              <span className="min-w-0 flex-1 truncate">{d.name}</span>
              <span className="text-[11px] font-bold text-muted-foreground">{d.count} 篇</span>
              <span className="text-muted-foreground">›</span>
            </div>
          ))}
          {dirNotes.map((n) => noteRow(n, false))}
          {!childDirs.length && !dirNotes.length && (
            <div className="m-3.5 border-3 border-dashed border-foreground bg-card/60 px-5 py-8 text-center">
              <div className="text-sm font-extrabold">这个目录还没有笔记</div>
              <p className="mt-1.5 text-xs font-semibold text-muted-foreground">在这里新建一篇，或让 AI 写进来</p>
              <Button size="sm" className="mt-3" onClick={() => setDlgNew(true)}>＋ 在这里新建</Button>
            </div>
          )}
        </>
      )}
    </div>
  )

  return (
    <div className="space-y-3">
      <div className="anim-fade-up flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-xl font-extrabold tracking-tight">知识库</h1>
          <p className="mt-0.5 text-xs font-semibold text-muted-foreground">仓库 &gt; 文件夹 &gt; 笔记 · 纯 MD · AI 与 Web 同源</p>
        </div>
        <Button size="sm" variant="outline" className="lg:hidden" onClick={() => setTreeDrawer(true)}>目录</Button>
      </div>

      <div className="grid h-[calc(100vh-215px)] min-h-[460px] grid-cols-1 gap-3.5 md:grid-cols-[260px_minmax(0,1fr)]">
        {/* 左：目录树（lg+ 常驻；<lg 抽屉） */}
        <div className="hidden min-h-0 flex-col overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)] lg:flex">
          {treePane}
        </div>

        {/* 右：列表（点行 = 弹浮窗；原右栏阅读/编辑区已取消 → 全站统一浮窗） */}
        <div className="flex min-h-0 flex-col overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
          {listHead}
          {listBody}
        </div>
      </div>

      {/* 移动端目录抽屉 */}
      {treeDrawer && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="absolute inset-0 bg-foreground/45" onClick={() => setTreeDrawer(false)} />
          <div className="absolute inset-y-0 left-0 flex w-[280px] max-w-[85vw] flex-col overflow-hidden border-r-3 border-foreground bg-card shadow-[6px_0_0px_var(--shadow-color)]">
            {treePane}
          </div>
        </div>
      )}

      {/* 新建 */}
      <NewNoteDialog
        open={dlgNew}
        onClose={() => setDlgNew(false)}
        prefillDir={dirPath}
        repoName={curRepo?.name ?? ''}
        exists={(p) => (notes ?? []).some((n) => n.path === p)}
        onCreate={doCreate}
      />

      {/* 仓库/目录权限（AI Key 可见性白名单；保存制） */}
      <NotePermDialog
        open={permDlg !== null}
        onOpenChange={(v) => { if (!v) setPermDlg(null) }}
        repoId={curRepoId ?? 0}
        repoName={curRepo?.name ?? ''}
        folder={permDlg?.folder ?? ''}
        rules={permRules}
        onSaved={(rules) => {
          setPermRules(rules)
          toast('权限已保存')
        }}
      />

      {/* 仓库管理（建/改/删；上限与删除守卫由后端裁决） */}
      <RepoMgmtDialog
        open={mgmtOpen}
        onClose={() => setMgmtOpen(false)}
        repos={repos ?? []}
        permRules={permRules}
        onRename={(r) => setRepoForm({ mode: 'rename', repo: r })}
        onChanged={(list) => {
          setRepos(list)
          if (curRepoId != null && !list.some((x) => x.id === curRepoId)) {
            const def = list.find((x) => x.is_default === 1) ?? list[0]
            if (def) switchRepo(def.id)
          }
        }}
        onGoPerm={(repoId) => {
          // 管理行「权限」：切到该仓库并打开仓库级权限
          switchRepo(repoId)
          setPermDlg({ folder: '' })
        }}
      />

      {/* 新建 / 改名仓库 */}
      <RepoFormDialog
        spec={repoForm}
        onClose={() => setRepoForm(null)}
        onSaved={(list, newId) => {
          setRepos(list)
          if (newId != null) switchRepo(newId)
        }}
      />
    </div>
  )
}

/* ================= 仓库管理弹窗 ================= */
function RepoMgmtDialog({ open, onClose, repos, permRules, onRename, onChanged, onGoPerm }: {
  open: boolean
  onClose: () => void
  repos: RepoItem[]
  permRules: NotePermRule[]
  onRename: (r: RepoItem) => void
  onChanged: (list: RepoItem[]) => void
  onGoPerm: (repoId: number) => void
}) {
  const [err, setErr] = useState('')
  const [busyId, setBusyId] = useState<number | null>(null)

  const reload = async () => {
    try {
      const r = await reposApi.list()
      onChanged(r.repos ?? [])
      return r.repos ?? []
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '刷新失败')
      return repos
    }
  }

  const doDelete = async (r: RepoItem) => {
    setErr('')
    setBusyId(r.id)
    try {
      await reposApi.remove(r.id)
      const list = await reload()
      const still = list.some((x) => x.id === r.id)
      if (!still) toast(`已删除仓库「${r.name}」（笔记不受影响）`)
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '删除失败')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) { setErr(''); onClose() } }}>
      <DialogContent className="sm:max-w-[620px]" data-role="repo-mgmt">
        <DialogHeader>
          <DialogTitle>管理仓库</DialogTitle>
        </DialogHeader>
        {err && (
          <div className="rounded-lg border-2 border-foreground bg-destructive px-3 py-2 text-xs font-bold text-destructive-foreground" data-role="mgmt-err">
            ⚠ {err}
          </div>
        )}
        <div className="max-h-[46vh] overflow-y-auto">
          {repos.map((r, i) => {
            const locked = permRules.some((x) => x.repo_id === r.id && x.folder_path === '' && x.mode === 'allow')
            return (
              <div key={r.id} className={cn('flex items-center gap-2.5 px-1 py-3', i > 0 && 'border-t-2 border-foreground/15')}>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5 text-[13.5px] font-extrabold">
                    <span className="min-w-0 truncate">{r.name}</span>
                    {r.is_default === 1 && <span className="shrink-0 rounded border-2 border-foreground bg-[var(--neon-lime)] px-1 text-[10px] font-bold">默认</span>}
                    {locked && <span className="shrink-0 text-muted-foreground">{LOCK_ICON}</span>}
                  </span>
                  {r.description && <span className="mt-0.5 block truncate text-[11px] font-semibold text-muted-foreground">{r.description}</span>}
                </span>
                <span className="shrink-0 rounded border-2 border-foreground bg-background px-1.5 text-[11px] font-bold">{r.note_count} 篇</span>
                <span className="flex shrink-0 gap-1.5">
                  <Button size="sm" variant="outline" data-role="repo-perm" onClick={() => onGoPerm(r.id)}>权限</Button>
                  <Button size="sm" variant="outline" onClick={() => onRename(r)}>改名</Button>
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={r.is_default === 1 || busyId === r.id}
                    data-role="repo-del"
                    title={r.is_default === 1 ? '默认仓库不可删除' : undefined}
                    onClick={() => void doDelete(r)}
                  >
                    删除
                  </Button>
                </span>
              </div>
            )
          })}
        </div>
        <p className="text-[11.5px] font-semibold text-muted-foreground">删除只删仓库本身，不删笔记——非空仓库须先移走或清空笔记；默认仓库不可删除。</p>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ================= 新建 / 改名仓库弹窗 ================= */
function RepoFormDialog({ spec, onClose, onSaved }: {
  spec: { mode: 'new' } | { mode: 'rename'; repo: RepoItem } | null
  onClose: () => void
  onSaved: (list: RepoItem[], newId: number | null) => void
}) {
  const isRename = spec?.mode === 'rename'
  const [name, setName] = useState('')
  const [desc, setDesc] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (spec) {
      setName(spec.mode === 'rename' ? spec.repo.name : '')
      setDesc(spec.mode === 'rename' ? spec.repo.description : '')
      setErr('')
      setBusy(false)
    }
  }, [spec])

  const submit = async () => {
    if (!name.trim()) { setErr('仓库名不能为空'); return }
    setBusy(true)
    setErr('')
    try {
      if (isRename && spec?.mode === 'rename') {
        await reposApi.update(spec.repo.id, { name: name.trim(), description: desc })
        const r = await reposApi.list()
        onSaved(r.repos ?? [], null)
        toast('已改名')
      } else {
        const r = await reposApi.create(name.trim(), desc)
        const list = await reposApi.list()
        onSaved(list.repos ?? [], r.repo.id)
        toast(`已创建仓库「${r.repo.name}」（默认对 AI 开放）`)
      }
      onClose()
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '保存失败，请重试')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={spec !== null} onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="sm:max-w-[460px]" data-role="repo-form">
        <DialogHeader>
          <DialogTitle>{isRename ? `改名 · ${spec?.mode === 'rename' ? spec.repo.name : ''}` : '新建仓库'}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3.5">
          <div className="space-y-1.5">
            <Label>仓库名称</Label>
            <Input data-role="repo-name" value={name} placeholder="如：工作库 / 学习库" onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label>描述（可选）</Label>
            <Input value={desc} placeholder="这个仓库放什么" onChange={(e) => setDesc(e.target.value)} />
          </div>
          <div className="flex items-start gap-2 rounded-lg border-2 border-foreground bg-[var(--neon-yellow)]/60 px-2.5 py-2 text-xs font-bold leading-relaxed">
            {isRename
              ? '📌 只改仓库名，仓库内的目录与笔记不受影响。'
              : '📌 新仓库默认对 AI 开放，创建后可在「权限」里配白名单；同一账号下仓库名不能重复。'}
          </div>
          {err && <p className="text-xs font-bold text-destructive" data-role="repo-err">{err}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button data-role="repo-save" disabled={busy} onClick={submit}>{busy ? '保存中…' : isRename ? '保存' : '创建'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ================= 新建弹窗 ================= */
function NewNoteDialog({ open, onClose, prefillDir, repoName, exists, onCreate }: {
  open: boolean
  onClose: () => void
  prefillDir: string
  repoName: string
  exists: (p: string) => boolean
  onCreate: (input: { path: string; title: string; tags: string; content: string }) => Promise<void>
}) {
  const [path, setPath] = useState('')
  const [title, setTitle] = useState('')
  const [tags, setTags] = useState('')
  const [content, setContent] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (open) {
      setPath(prefillDir ? `${prefillDir}/` : '')
      setTitle('')
      setTags('')
      setContent('')
      setErr('')
      setBusy(false)
    }
  }, [open, prefillDir])

  async function submit() {
    const p = normPath(path)
    const pe = pathError(path.trim())
    if (pe) return setErr(pe)
    if (exists(p)) return setErr('该路径已存在——可在列表打开它编辑，或换个名字')
    setBusy(true)
    try {
      await onCreate({ path: p, title, tags, content })
      onClose()
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent data-role="dlg-new" className="max-w-[660px]">
        <DialogHeader>
          <DialogTitle>新建笔记</DialogTitle>
        </DialogHeader>
        <div className="space-y-3.5">
          <div className="space-y-1.5">
            <Label>路径</Label>
            <Input data-role="nw-path" value={path} placeholder="ai/prompt/写作规范.md" onChange={(e) => setPath(e.target.value)} />
            <p className="text-[11.5px] font-semibold text-muted-foreground">保存到仓库「{repoName}」；用 / 分层即自动建目录；末段缺扩展名会自动补 .md</p>
          </div>
          <div className="flex gap-3">
            <div className="min-w-0 flex-1 space-y-1.5">
              <Label>标题（可选）</Label>
              <Input data-role="nw-title" value={title} placeholder="缺省取文件名" onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div className="min-w-0 flex-1 space-y-1.5">
              <Label>标签（逗号分隔）</Label>
              <Input value={tags} placeholder="ai, writing" onChange={(e) => setTags(e.target.value)} />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label>内容（Markdown，留空自动生成标题行）</Label>
            <Textarea data-role="nw-content" rows={7} value={content} placeholder={'# 标题\n\n正文…'} onChange={(e) => setContent(e.target.value)} className="font-mono text-[12.5px]" />
          </div>
          {err && <p className="text-xs font-bold text-destructive">{err}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button data-role="nw-submit" disabled={busy} onClick={submit}>创建笔记</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ================= 重命名 / 移动弹窗 ================= */
