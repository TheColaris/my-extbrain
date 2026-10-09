import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ApiError, keysApi, notePermApi, type APIKeyItem, type NotePermRule } from '@/lib/api'
import { cn } from '@/lib/utils'

// 仓库/目录权限弹窗（对 AI 生效，网页面板不受影响）：
// 模式分段（不限 / 白名单）+ Key 勾选（勾选暂存 + 底部保存制——多选弹窗红线）。
// 语义：未配置=开放；白名单空=对 AI 完全封闭；最深规则优先（仓库规则=本仓库默认，目录可再收窄）。
export function NotePermDialog({ open, onOpenChange, repoId, repoName, folder, rules, onSaved }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  repoId: number
  repoName: string
  folder: string // '' = 仓库级默认规则
  rules: NotePermRule[] // 全量规则（组件内按 repoId 过滤）
  onSaved: (rules: NotePermRule[]) => void
}) {
  const [mode, setMode] = useState<'open' | 'allow'>('open')
  const [picked, setPicked] = useState<Set<number>>(new Set())
  const [keys, setKeys] = useState<APIKeyItem[]>([])
  const [inheritFrom, setInheritFrom] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const repoRules = rules.filter((r) => r.repo_id === repoId)

  // 打开时：拉 Key 列表 + 预填（本目录规则优先；未单独配置则预览最近已配置祖先——含仓库级规则）
  useEffect(() => {
    if (!open) return
    setErr('')
    keysApi.list().then((r) => setKeys(r.keys ?? [])).catch(() => setKeys([]))

    const own = repoRules.find((r) => r.folder_path === folder)
    const segs = folder ? folder.split('/') : []
    let from: string | null = null
    let base: NotePermRule | null = own ?? null
    if (!base) {
      for (let i = segs.length - 1; i >= 0; i--) {
        const p = segs.slice(0, i).join('/')
        const r = repoRules.find((x) => x.folder_path === p)
        if (r) { from = p; base = r; break }
      }
    }
    setInheritFrom(!own && from !== null ? from : null)
    if (base && base.mode === 'allow') {
      setMode('allow')
      setPicked(new Set(base.key_ids.map(Number)))
    } else {
      setMode('open')
      setPicked(new Set())
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, repoId, folder, rules])

  const toggle = (id: number) => {
    setPicked((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const save = async () => {
    setBusy(true)
    setErr('')
    try {
      const r = await notePermApi.put({ repo_id: repoId, folder_path: folder, mode, key_ids: [...picked] })
      onSaved(r.rules)
      onOpenChange(false)
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '保存失败，请重试')
    } finally {
      setBusy(false)
    }
  }

  const isRepo = folder === ''
  const label = isRepo ? `仓库权限 · ${repoName}` : `目录权限 · ${folder}/`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[460px]" data-role="perm-dialog" data-tour="perm-dialog">
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
        </DialogHeader>

        <div className="text-xs font-semibold text-muted-foreground">
          作用于 <code className="rounded border-2 border-foreground bg-[var(--neon-yellow)] px-1 font-mono text-[11px] font-bold">{isRepo ? repoName : `${repoName} / ${folder}/`}</code>{' '}
          全部笔记（含子目录）。对 AI 生效，网页面板不受影响。
        </div>

        {isRepo && (
          <div className="flex items-start gap-2 rounded-lg border-2 border-foreground bg-[var(--neon-yellow)]/60 px-2.5 py-2 text-xs font-bold leading-relaxed">
            📌 仓库规则 = 本仓库的默认可见性；子目录可再单独收窄（以最深规则为准）。
          </div>
        )}
        {inheritFrom !== null && (
          <div className="flex items-start gap-2 rounded-lg border-2 border-foreground bg-[var(--neon-yellow)]/60 px-2.5 py-2 text-xs font-bold leading-relaxed">
            ↳ 该目录未单独配置，当前继承「{inheritFrom === '' ? `${repoName}（仓库规则）` : inheritFrom + '/'}」的白名单（以最深规则为准）；保存后以本目录为准。
          </div>
        )}

        <div className="mt-1 text-xs font-extrabold">可见性模式</div>
        <div className="flex overflow-hidden rounded-lg border-2 border-foreground" data-role="perm-mode">
          {([['open', '不限 · 所有 AI 可见'], ['allow', '白名单 · 仅勾选可见']] as const).map(([m, text]) => (
            <button
              key={m}
              type="button"
              className={cn(
                'flex-1 px-2 py-1.5 text-xs font-bold transition-shadow',
                m === 'allow' && 'border-l-2 border-foreground',
                mode === m ? 'bg-[var(--neon-yellow)]' : 'bg-card hover:bg-background',
              )}
              onClick={() => setMode(m)}
            >
              {text}
            </button>
          ))}
        </div>

        <div className={cn('mt-1 text-xs font-extrabold', mode !== 'allow' && 'opacity-50')}>可访问的 Key</div>
        <div className={cn('overflow-hidden rounded-xl border-2 border-foreground', mode !== 'allow' && 'pointer-events-none opacity-50')}>
          {keys.length === 0 && <div className="bg-card px-3 py-3 text-xs font-semibold text-muted-foreground">还没有 API Key——先到「API 密钥」签发</div>}
          {keys.map((k, i) => {
            const canKB = k.scope === 'all' || k.scope === 'notes'
            const checked = picked.has(k.id)
            return (
              <label
                key={k.id}
                className={cn(
                  'flex cursor-pointer items-center gap-2.5 bg-card px-3 py-2.5 hover:bg-background',
                  i > 0 && 'border-t-2 border-foreground',
                  !canKB && 'cursor-not-allowed opacity-50',
                )}
              >
                <input
                  type="checkbox"
                  className="h-4 w-4 shrink-0 accent-foreground"
                  checked={checked}
                  disabled={!canKB}
                  onChange={() => toggle(k.id)}
                />
                <span className="min-w-[76px] text-[13px] font-extrabold">{k.key_name}</span>
                <span className="min-w-0 flex-1 truncate font-mono text-[11px] font-semibold text-muted-foreground">{k.key_hint}</span>
                {canKB ? (
                  <span className="shrink-0 rounded-md border-2 border-foreground bg-[var(--neon-blue)] px-1.5 py-px text-[10.5px] font-bold">scope {k.scope}</span>
                ) : (
                  <span className="shrink-0 rounded-md border-2 border-foreground bg-background px-1.5 py-px text-[10.5px] font-bold">无知识库权限</span>
                )}
              </label>
            )
          })}
        </div>
        {mode === 'allow' && picked.size === 0 && (
          <div className="text-[11.5px] font-bold text-muted-foreground">未勾选任何 Key = 对 AI 完全封闭（网页面板不受影响）。</div>
        )}

        {err && <div className="text-xs font-bold text-red-600">{err}</div>}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
          <Button data-role="perm-save" disabled={busy} onClick={save}>{busy ? '保存中…' : '保存'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
