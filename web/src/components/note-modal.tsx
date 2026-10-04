import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import MDEditor from '@uiw/react-md-editor'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { MdView } from '@/components/md-view'
import { BrandMark } from '@/components/brand-mark'
import { ShareDialog } from '@/components/share-dialog'
import { undoToast } from '@/components/undo-toast'
import { ApiError, notesApi, type NoteFull } from '@/lib/api'
import { downloadMd } from '@/lib/download'
import { cn } from '@/lib/utils'

// 笔记浮窗：
// 全站唯一笔记卡——库内点行、搜索结果、日志入口、单篇直达 /notes/<路径> 全用它。
// 卡自含：拉取 / 阅读 / 编辑（⌘Enter 保存 · Esc 取消 · 未保存点）/ 409 冲突弹窗 /
// 重命名·移动 / 删除+撤销 / 复制。删除·恢复·重命名·保存后派发 'extbrain:notes-changed'
// 让库页刷新列表。关闭语义：× / esc（阅读态）→ 调 onClose —— NoteModal 移除 ?note= 参数
// （打开走 push，浏览器返回键天然关闭），NoteSoloView 回 /notes；编辑态 Esc = 取消编辑。

/** 按段编码路径（'/' 保留字面量），用于构造 /notes/<path> 形态的路由链接 */
export function encNotePath(p: string): string {
  return p.split('/').map(encodeURIComponent).join('/')
}

/** 通知库页刷新列表（删除/恢复/重命名/保存后） */
function notifyNotesChanged() {
  window.dispatchEvent(new Event('extbrain:notes-changed'))
}

export function NoteModal() {
  const [sp, setSp] = useSearchParams()
  const path = sp.get('note') ?? ''
  const repo = sp.get('repo') ?? '' // 所属仓库（缺省=默认仓库；仓库化后搜索/日志入口带回）
  const open = path !== ''

  const close = () => {
    const next = new URLSearchParams(sp)
    next.delete('note')
    next.delete('repo')
    setSp(next, { replace: true })
  }
  const changePath = (p: string) => {
    const next = new URLSearchParams(sp)
    next.set('note', p)
    setSp(next, { replace: true })
  }

  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-foreground/45 p-6 max-md:p-2.5" onClick={close}>
      <NoteModalCard path={path} repo={repo} onClose={close} onPathChange={changePath} />
    </div>
  )
}

/** 单篇直达视图：/notes/<笔记路径>（不透明全屏遮罩，只呈现这一张卡；关闭回知识库） */
export function NoteSoloView({ path, initialMode }: { path: string; initialMode?: 'read' | 'edit' }) {
  const nav = useNavigate()
  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-background">
      <div className="relative z-10 flex items-center gap-2.5 px-7 py-5">
        <button
          type="button" aria-label="回主页" onClick={() => nav('/')}
          className="bk-interactive flex h-[30px] w-[30px] cursor-pointer items-center justify-center rounded-[7px] border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)] hover:shadow-[5px_5px_0px_var(--shadow-color)]"
        >
          <BrandMark className="h-[17px] w-[17px]" />
        </button>
        <span className="text-sm font-extrabold">我的外脑</span>
        <span className="font-mono text-[11px] font-semibold text-muted-foreground">my-extbrain</span>
        <button
          type="button" onClick={() => nav('/notes')}
          className="ml-auto flex h-9 cursor-pointer items-center gap-2 rounded-lg border-3 border-foreground bg-card px-3 text-[13px] font-bold shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)]"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-4 w-4"><path d="M4 4h7v16H4z" /><path d="M11 4h9v16h-9" /><path d="M14 9h3M14 13h3" /></svg>
          打开知识库
        </button>
      </div>
      <div className="flex min-h-[calc(100vh-70px)] items-start justify-center p-6 max-md:p-3">
        <NoteModalCard
          path={path} initialMode={initialMode}
          onClose={() => nav('/notes')}
          onPathChange={(p) => nav('/notes/' + encNotePath(p), { replace: true })}
        />
      </div>
    </div>
  )
}

/** 笔记卡（阅读态 ⇄ 编辑态同卡切换；两种浮窗形态共用） */
function NoteModalCard({ path, repo, initialMode = 'read', onClose, onPathChange }: {
  path: string
  initialMode?: 'read' | 'edit'
  repo?: string
  onClose: () => void
  onPathChange: (p: string) => void
}) {
  const [phase, setPhase] = useState<'loading' | 'done' | 'missing'>('loading')
  const [note, setNote] = useState<NoteFull | null>(null)
  const [mode, setMode] = useState<'read' | 'edit'>('read')
  const [draft, setDraft] = useState({ title: '', tags: '', content: '' })
  const [edTab, setEdTab] = useState<'edit' | 'preview' | 'live'>('edit')
  const [dirty, setDirty] = useState(false)
  const [conflict, setConflict] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const [rnTo, setRnTo] = useState('')
  const [rnErr, setRnErr] = useState('')
  const [rnBusy, setRnBusy] = useState(false)
  const [sharing, setSharing] = useState(false)

  useEffect(() => {
    let alive = true
    setPhase('loading')
    setNote(null)
    setMode('read')
    setDirty(false)
    setConflict(false)
    notesApi
      .get(path, false, repo || undefined)
      .then((n) => {
        if (!alive) return
        setNote(n)
        setPhase('done')
        if (initialMode === 'edit') {
          setDraft({ title: n.title, tags: (n.tags ?? []).join(', '), content: n.content })
          setMode('edit')
          setEdTab('edit')
        }
      })
      .catch(() => { if (alive) setPhase('missing') })
    return () => { alive = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, repo])

  const toEdit = () => {
    if (!note) return
    setDraft({ title: note.title, tags: (note.tags ?? []).join(', '), content: note.content })
    setMode('edit')
    setEdTab('edit')
    setDirty(false)
  }
  const cancelEdit = () => { setMode('read'); setDirty(false) }

  const saveEdit = async (force = false) => {
    if (!note) return
    const title = draft.title.trim() || (path.split('/').pop() ?? path)
    const tags = draft.tags.split(/[,，]/).map((t) => t.trim()).filter(Boolean)
    try {
      const r = await notesApi.put(path, {
        content: draft.content, title, tags,
        ...(force ? {} : { expected_hash: note.content_hash }),
      }, repo || undefined)
      setNote({ ...note, title, tags, content: draft.content, content_hash: r.content_hash, update_time: new Date().toISOString() })
      setMode('read')
      setDirty(false)
      setConflict(false)
      notifyNotesChanged()
      undoToast('已保存')
    } catch (e) {
      if (e instanceof ApiError && e.code === 'hash_conflict') { setConflict(true); return }
      undoToast(e instanceof ApiError ? e.message : '保存失败')
    }
  }

  const doDelete = async () => {
    if (!note) return
    const title = note.title
    try {
      await notesApi.del(path, repo || undefined)
    } catch { /* 幂等：失败也走撤销提示，恢复兜底 */ }
    onClose()
    notifyNotesChanged()
    undoToast(`已删除「${title}」`, {
      label: '撤销',
      onAction: async () => {
        try {
          await notesApi.restore(path, repo || undefined)
          notifyNotesChanged()
          undoToast('已恢复')
        } catch (e) {
          undoToast(e instanceof ApiError ? e.message : '恢复失败')
        }
      },
    })
  }

  const doRename = async () => {
    const t = rnTo.trim().replace(/^\/+/, '')
    if (!t) { setRnErr('路径不能为空'); return }
    if (t === path) { setRnErr('路径没有变化'); return }
    const to = /\.md$/i.test(t) ? t : t + '.md'
    setRnBusy(true)
    try {
      const r = await notesApi.move(path, to, repo || undefined)
      setRenaming(false)
      notifyNotesChanged()
      undoToast(`已移动到 ${r.note.path}`)
      onPathChange(r.note.path)
    } catch (e) {
      setRnErr(e instanceof ApiError ? e.message : '移动失败')
    } finally {
      setRnBusy(false)
    }
  }

  // esc：弹窗层 > 编辑态取消 > 关窗（阅读态）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (conflict || renaming || sharing) return // Radix Dialog 自己处理 Esc
      if (mode === 'edit') { setMode('read'); setDirty(false); return }
      onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, conflict, renaming, sharing])

  const tabs: ['edit' | 'preview' | 'live', string][] = [['edit', '编辑'], ['preview', '预览'], ['live', '分屏']]

  return (
    <div
      role="dialog" aria-label="笔记浮窗"
      onClick={(e) => e.stopPropagation()}
      className="flex max-h-[85vh] w-[min(880px,94vw)] flex-col overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[8px_8px_0px_var(--shadow-color)] max-md:max-h-[92vh]"
    >
      {/* 黄码头：mono 路径 + 编辑中徽章 + × */}
      <div className="flex shrink-0 items-center gap-3 border-b-3 border-foreground bg-primary px-4 py-3">
        <span className="min-w-0 flex-1 break-all font-mono text-[13.5px] font-extrabold leading-relaxed">{path}</span>
        {mode === 'edit' && (
          <span className="inline-flex h-5 shrink-0 items-center rounded border-2 border-foreground bg-background px-1.5 text-[10.5px] font-extrabold">编辑中</span>
        )}
        <button
          type="button" aria-label="关闭" onClick={onClose} title="关闭（esc）"
          className="flex h-[30px] w-[30px] shrink-0 cursor-pointer items-center justify-center rounded-lg border-2 border-foreground bg-background shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[3px_3px_0px_var(--shadow-color)]"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" className="h-3.5 w-3.5"><path d="M6 6l12 12M18 6 6 18" /></svg>
        </button>
      </div>

      {/* 元信息行（阅读态） */}
      {phase === 'done' && note && mode === 'read' && (
        <div className="flex shrink-0 flex-wrap items-center gap-2.5 border-b-2 border-foreground px-4 py-2 text-[11.5px] font-semibold text-muted-foreground">
          <span>更新 {fmtFull(note.update_time)}</span>
          <span>{fmtSize(note.content.length)}</span>
          <span className="font-mono">hash {note.content_hash.slice(0, 8)}</span>
          {(note.tags ?? []).map((t) => (
            <span key={t} className="inline-flex h-[17px] items-center rounded border-[1.5px] border-foreground bg-background px-1 text-[10.5px] font-bold text-foreground">{t}</span>
          ))}
        </div>
      )}

      {/* 正文 / 编辑区 */}
      <div className="min-h-[120px] flex-1 overflow-y-auto">
        {phase === 'loading' && (
          <div className="px-6 py-5 max-md:px-4">
            <span className="mb-3 block h-[17px] w-[38%] rounded bg-foreground/10" />
            <span className="mb-3 block h-3 w-[22%] rounded bg-foreground/10" />
            <div className="h-3.5" />
            <span className="mb-3 block h-3 w-[92%] rounded bg-foreground/10" />
            <span className="mb-3 block h-3 w-[86%] rounded bg-foreground/10" />
            <span className="block h-3 w-[64%] rounded bg-foreground/10" />
          </div>
        )}
        {phase === 'done' && note && mode === 'read' && (
          <div className="px-6 py-5 max-md:px-4"><MdView content={note.content} className="rounded-none border-0 p-0 shadow-none" /></div>
        )}
        {phase === 'done' && note && mode === 'edit' && (
          <div
            className="px-4 py-3.5"
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); void saveEdit() }
            }}
          >
            <div className="mb-2.5 flex gap-2.5 max-md:flex-col">
              <div className="min-w-0 flex-1 space-y-1.5">
                <Label>标题</Label>
                <Input data-role="ed-title" value={draft.title} onChange={(e) => { setDraft((d) => ({ ...d, title: e.target.value })); setDirty(true) }} />
              </div>
              <div className="min-w-0 flex-1 space-y-1.5">
                <Label>标签（逗号分隔）</Label>
                <Input value={draft.tags} placeholder="ai, writing" onChange={(e) => { setDraft((d) => ({ ...d, tags: e.target.value })); setDirty(true) }} />
              </div>
            </div>
            <div className="mb-2.5 inline-flex overflow-hidden border-3 border-foreground bg-background shadow-[2px_2px_0px_var(--shadow-color)]">
              {tabs.map(([k, label], i) => (
                <button
                  key={k} type="button"
                  className={cn('px-3.5 py-1 text-xs font-bold', i > 0 && 'border-l-2 border-foreground', edTab === k ? 'bg-primary' : 'hover:bg-accent')}
                  onClick={() => setEdTab(k)}
                >
                  {label}
                </button>
              ))}
            </div>
            <div data-color-mode="light">
              {edTab === 'preview' ? (
                <MdView content={draft.content} />
              ) : (
                <MDEditor
                  value={draft.content}
                  onChange={(v?: string) => { setDraft((d) => ({ ...d, content: v ?? '' })); setDirty(true) }}
                  height={400}
                  preview={edTab === 'live' ? 'live' : 'edit'}
                  visiableDragbar={false}
                  textareaProps={{ style: { fontSize: 13, lineHeight: '1.7' } }}
                />
              )}
            </div>
          </div>
        )}
        {phase === 'missing' && (
          <div className="py-11 text-center">
            <div className="mx-auto mb-3.5 flex h-[52px] w-[52px] items-center justify-center rounded-xl border-3 border-foreground bg-[var(--neon-pink)] shadow-[4px_4px_0px_var(--shadow-color)]">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" className="h-6 w-6"><circle cx="12" cy="12" r="9" /><path d="M9.4 9.2a2.7 2.7 0 0 1 5.2 1c0 1.8-2.6 2.2-2.6 3.8" /><circle cx="12" cy="17.6" r="0.4" fill="currentColor" /></svg>
            </div>
            <div className="text-[15px] font-extrabold">笔记不存在或已删除</div>
            <div className="mt-1.5 break-all font-mono text-[12.5px] font-semibold text-muted-foreground">{path} · 可能在回收站里</div>
            <div className="mt-4 flex justify-center gap-2.5">
              <Button size="sm" variant="outline" onClick={() => window.location.assign('/trash')}>去回收站找</Button>
              <Button size="sm" onClick={onClose}>关闭</Button>
            </div>
          </div>
        )}
      </div>

      {/* 底部动作条：阅读态 */}
      {mode === 'read' && (
        <div className="flex shrink-0 flex-wrap items-center gap-2.5 border-t-2 border-foreground bg-background px-4 py-3">
          <span className="mr-auto hidden items-center gap-2 text-[11px] font-semibold text-muted-foreground sm:flex">
            <span className="rounded border-2 border-foreground bg-card px-1.5 font-mono text-[10px] font-semibold shadow-[2px_2px_0px_var(--shadow-color)]">esc</span> 关闭
          </span>
          <Button
            size="sm" variant="outline" data-role="note-download" className="normal-case" disabled={phase !== 'done'}
            onClick={() => note && downloadMd(path.split('/').pop() ?? path, note.content)}
          >下载 .md</Button>
          <Button size="sm" variant="outline" data-role="share-btn" disabled={phase !== 'done'} onClick={() => setSharing(true)}>分享</Button>
          <Button size="sm" variant="outline" disabled={phase !== 'done'} onClick={() => { setRnTo(path); setRnErr(''); setRenaming(true) }}>重命名 / 移动</Button>
          <Button size="sm" variant="destructive" disabled={phase !== 'done'} onClick={doDelete}>删除</Button>
          <Button size="sm" disabled={phase !== 'done'} onClick={toEdit}>编辑</Button>
        </div>
      )}
      {/* 底部动作条：编辑态 */}
      {mode === 'edit' && (
        <div className="flex shrink-0 flex-wrap items-center gap-2.5 border-t-2 border-foreground bg-background px-4 py-3">
          {dirty && (
            <span className="mr-auto flex items-center gap-1.5 text-[11.5px] font-bold text-[#8a5a00]">
              <span className="h-[9px] w-[9px] rounded-full border-2 border-foreground bg-primary" />未保存
            </span>
          )}
          <span className={cn('flex items-center gap-2 text-[11px] font-semibold text-muted-foreground', !dirty && 'mr-auto')}>
            <span className="rounded border-2 border-foreground bg-card px-1.5 font-mono text-[10px] font-semibold shadow-[2px_2px_0px_var(--shadow-color)]">esc</span> 取消
          </span>
          <Button size="sm" variant="outline" onClick={cancelEdit}>取消</Button>
          <Button size="sm" data-role="save" onClick={() => void saveEdit()}>保存</Button>
        </div>
      )}

      {/* 分享（链接 / 二维码 / 图片） */}
      <ShareDialog open={sharing} onOpenChange={setSharing} path={path} note={note} />

      {/* 重命名 / 移动 */}
      <Dialog open={renaming} onOpenChange={(v) => !v && setRenaming(false)}>
        <DialogContent data-role="dlg-rename" className="max-w-[520px]">
          <DialogHeader><DialogTitle>重命名 / 移动</DialogTitle></DialogHeader>
          <div className="space-y-3.5">
            <div className="space-y-1.5">
              <Label>当前路径</Label>
              <div className="break-all border-2 border-foreground bg-background px-3 py-2 font-mono text-xs font-semibold">{path}</div>
            </div>
            <div className="space-y-1.5">
              <Label>新路径</Label>
              <Input data-role="rn-to" value={rnTo} onChange={(e) => setRnTo(e.target.value)} />
              <p className="text-[11.5px] font-semibold text-muted-foreground">改文件名 = 重命名；改目录 = 移动；目录不存在会自动创建</p>
            </div>
            {rnErr && <p className="text-xs font-bold text-destructive">{rnErr}</p>}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenaming(false)}>取消</Button>
            <Button data-role="rn-submit" disabled={rnBusy} onClick={doRename}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 覆盖冲突（编辑期间 AI/CLI 也写入） */}
      <Dialog open={conflict} onOpenChange={(v) => !v && setConflict(false)}>
        <DialogContent className="max-w-[520px]">
          <DialogHeader><DialogTitle>内容冲突</DialogTitle></DialogHeader>
          <p className="text-sm font-semibold leading-relaxed">
            这篇笔记在你编辑期间被其他端修改（可能是 AI / CLI 写入）。直接保存会覆盖对方的改动。
          </p>
          <div className="break-all border-2 border-foreground bg-background px-3 py-2 font-mono text-xs font-semibold">{path}</div>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => void saveEdit(true)}>用我的版本覆盖</Button>
            <Button
              onClick={async () => {
                try {
                  const n = await notesApi.get(path)
                  setNote(n)
                  setConflict(false)
                  setMode('read')
                  setDirty(false)
                  undoToast('已载入最新版')
                } catch {
                  undoToast('载入失败')
                }
              }}
            >
              查看最新版
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function fmtFull(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getMonth() + 1}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`
  return `${(n / 1024).toFixed(1)} KB`
}
