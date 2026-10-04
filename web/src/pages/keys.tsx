import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { ApiError, keysApi, keysApiFull, type APIKeyItem, type IssueResp } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { KEY_SCOPE, type KeyScope } from '@/lib/enums'
import { cn } from '@/lib/utils'

const SCOPE_LABEL: Record<KeyScope, string> = {
  [KEY_SCOPE.all]: 'all · 全部',
  [KEY_SCOPE.todo]: 'todo · 待办与便签',
  [KEY_SCOPE.notes]: 'notes · 知识库',
}

function fmtTime(iso?: string): string {
  if (!iso) return '从未使用'
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function KeysPage() {
  const [keys, setKeys] = useState<APIKeyItem[] | null>(null)
  const [issued, setIssued] = useState<IssueResp | null>(null) // 签发一次性展示
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<APIKeyItem | null>(null)
  const [err, setErr] = useState('')

  const reload = () => keysApi.list().then((r) => setKeys(r.keys)).catch(() => setKeys([]))
  useEffect(() => {
    reload()
  }, [])

  return (
    <div className="space-y-6">
      <div className="anim-fade-up flex items-center justify-between">
        <div>
          <h1 className="text-xl font-extrabold tracking-tight">API 密钥</h1>
          <p className="mt-0.5 text-xs font-semibold text-muted-foreground">
            服务端只存哈希 · 完整 Key 仅签发时可见一次
          </p>
        </div>
        <Button onClick={() => { setErr(''); setCreateOpen(true) }}>＋ 新建密钥</Button>
      </div>

      {err && <p className="anim-pop border-3 border-foreground bg-destructive px-3 py-2 text-sm font-bold text-destructive-foreground">{err}</p>}

      {/* 签发成功态（一次性） */}
      {issued && <IssuedCard resp={issued} onClose={() => setIssued(null)} />}

      {/* 密钥表 */}
      <div className="anim-fade-up d-2 overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>权限</TableHead>
              <TableHead className="hidden md:table-cell">最近使用</TableHead>
              <TableHead className="hidden md:table-cell">创建</TableHead>
              <TableHead className="hidden lg:table-cell">到期</TableHead>
              <TableHead className="w-44" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(keys ?? []).map((k, i) => (
              <TableRow key={k.id} className="anim-slide-in" style={{ animationDelay: `${0.08 + i * 0.05}s` }}>
                <TableCell className="font-bold">{k.key_name}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{k.key_hint}</TableCell>
                <TableCell>
                  <ScopeBadge scope={k.scope} />
                </TableCell>
                <TableCell className="hidden whitespace-nowrap text-xs text-muted-foreground md:table-cell">{fmtTime(k.last_use_time)}</TableCell>
                <TableCell className="hidden whitespace-nowrap text-xs text-muted-foreground md:table-cell">{fmtTime(k.create_time)}</TableCell>
                  <TableCell className="hidden whitespace-nowrap text-xs lg:table-cell">
                    {k.expire_time ? (
                      <span className={new Date(k.expire_time) < new Date() ? 'font-bold text-destructive' : ''}>
                        {new Date(k.expire_time).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">永久</span>
                    )}
                  </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-2">
                    <Button variant="outline" size="sm" onClick={() => setEditing(k)}>编辑</Button>
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={async () => {
                        try {
                          await keysApiFull.revoke(k.id)
                          reload()
                        } catch (e) {
                          setErr(e instanceof ApiError ? e.message : '吊销失败')
                        }
                      }}
                    >
                      吊销
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {keys?.length === 0 && (
              <TableRow>
                <TableCell colSpan={7} className="py-8 text-center text-sm text-muted-foreground">
                  还没有密钥，点右上「新建密钥」签发第一个
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <CreateDialog open={createOpen} onClose={() => setCreateOpen(false)}
        onIssued={(r) => { setCreateOpen(false); setIssued(r); reload() }} />
      <EditDialog key={editing?.id} item={editing} onClose={() => setEditing(null)}
        onSaved={() => { setEditing(null); reload() }} />
    </div>
  )
}

function ScopeBadge({ scope }: { scope: KeyScope }) {
  const cls: Record<KeyScope, string> = {
    [KEY_SCOPE.all]: 'bg-primary',
    [KEY_SCOPE.todo]: 'bg-[var(--neon-blue)]',
    [KEY_SCOPE.notes]: 'bg-[var(--neon-purple)]',
  }
  return <span className={cn('border-3 border-foreground px-2 py-0.5 text-xs font-bold', cls[scope])}>{scope}</span>
}

/* 签发一次性展示卡（lime 底 + 完整 Key + CLI 命令 + 复制） */
function IssuedCard({ resp, onClose }: { resp: IssueResp; onClose: () => void }) {
  const [copied, setCopied] = useState<'key' | 'cmd' | null>(null)
  const copy = async (text: string, which: 'key' | 'cmd') => {
    if (await copyText(text)) {
      setCopied(which)
      setTimeout(() => setCopied(null), 1500)
    }
  }
  return (
    <div className="anim-pop rounded-xl border-3 border-foreground bg-[var(--neon-green)] p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
      <div className="flex items-center justify-between">
        <span className="text-sm font-extrabold">已创建「{resp.key.key_name}」 · 仅此一次展示</span>
        <ScopeBadge scope={resp.key.scope} />
      </div>
      <div className="mt-3 overflow-x-auto rounded-lg border-3 border-foreground bg-background p-3 font-mono text-xs">{resp.full_key}</div>
      <div className="mt-3 flex items-center justify-between">
        <span className="text-xs font-bold">CLI 配置（可直接复制给 AI）</span>
        <Button size="sm" variant="outline" className="bg-background" onClick={() => copy(resp.cli_command, 'cmd')}>
          {copied === 'cmd' ? '✓ 已复制' : '复制'}
        </Button>
      </div>
      <div className="mt-2 overflow-x-auto rounded-lg border-3 border-foreground bg-background p-3 font-mono text-xs">{resp.cli_command}</div>
      <div className="mt-3 flex justify-end gap-2">
        <Button size="sm" className="bg-background" onClick={() => copy(resp.full_key, 'key')}>
          {copied === 'key' ? '✓ 已复制' : '复制 Key'}
        </Button>
        <Button size="sm" onClick={onClose}>知道了</Button>
      </div>
    </div>
  )
}

const EXPIRE_OPTIONS = [
  { v: 0, label: '永不过期' },
  { v: 7, label: '7 天' },
  { v: 30, label: '30 天' },
  { v: 90, label: '90 天' },
]

function CreateDialog({ open, onClose, onIssued }: { open: boolean; onClose: () => void; onIssued: (r: IssueResp) => void }) {
  const [name, setName] = useState('')
  const [expire, setExpire] = useState(0)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  async function submit() {
    if (!name.trim()) {
      setErr('给密钥起个名字')
      return
    }
    setBusy(true)
    setErr('')
    try {
      onIssued(await keysApiFull.issue(name.trim(), expire))
      setName('')
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '签发失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建密钥</DialogTitle>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="kname">名称</Label>
          <Input id="kname" value={name} onChange={(e) => setName(e.target.value)} placeholder="如：工作机 / 备用机"
            onKeyDown={(e) => e.key === 'Enter' && submit()} />
          {err && <p className="text-sm font-bold text-destructive">{err}</p>}
          <div className="space-y-2">
            <Label>有效期</Label>
            <Select value={String(expire)} onValueChange={(v) => setExpire(Number(v))}>
              <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent>
                {EXPIRE_OPTIONS.map((o) => (
                  <SelectItem key={o.v} value={String(o.v)}>{o.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? <span className="bk-loader" /> : '签发'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function EditDialog({ item, onClose, onSaved }: { item: APIKeyItem | null; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(item?.key_name ?? '')
  const [scope, setScope] = useState<KeyScope>(item?.scope ?? KEY_SCOPE.all)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  if (!item) return null
  const target = item
  async function submit() {
    setBusy(true)
    setErr('')
    try {
      await keysApiFull.update(target.id, { name: name.trim(), scope })
      onSaved()
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={!!item} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>编辑密钥 · {item.key_name}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="ename">名称</Label>
            <Input id="ename" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label>权限</Label>
            <Select value={scope} onValueChange={(v) => setScope(v as KeyScope)}>
              <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent>
                {(Object.keys(KEY_SCOPE) as KeyScope[]).map((s) => (
                  <SelectItem key={s} value={s}>{SCOPE_LABEL[s]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {err && <p className="text-sm font-bold text-destructive">{err}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? <span className="bk-loader" /> : '保存'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
