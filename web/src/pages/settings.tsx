import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ApiError, accountApi, getToken, notifyApi, pushApi, setToken, type MeInfo, type NotifyChannel } from '@/lib/api'
import { urlBase64ToUint8Array } from '@/lib/push'
import { AVATAR_BG_OPTIONS, CHANNEL_TYPE } from '@/lib/enums'
import { UserAvatar, syncUserCache } from '@/components/user-avatar'
import { cn } from '@/lib/utils'

type Tab = 'account' | 'push' | 'bot'

function maskPhone(p?: string): string {
  if (!p) return ''
  return p.slice(0, 3) + '****' + p.slice(-4)
}

export function SettingsPage() {
  const [tab, setTab] = useState<Tab>('account')
  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="anim-fade-up">
        <h1 className="text-xl font-extrabold tracking-tight">设置</h1>
      </div>
      <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)}>
        <TabsList>
          <TabsTrigger value="account">账户</TabsTrigger>
          <TabsTrigger value="push">浏览器通知</TabsTrigger>
          <TabsTrigger value="bot">机器人通知</TabsTrigger>
        </TabsList>
      </Tabs>
      {tab === 'account' && <AccountTab />}
      {tab === 'push' && <PushTab />}
      {tab === 'bot' && <BotTab />}
    </div>
  )
}

/* ============ 账户 ============ */
function AccountTab() {
  const [me, setMe] = useState<MeInfo | null>(null)
  const [nick, setNick] = useState('')
  const [dlg, setDlg] = useState<'bind-phone' | 'bind-email' | 'pwd' | null>(null)
  const [avatarDlg, setAvatarDlg] = useState(false)
  const [exporting, setExporting] = useState(false)
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')

  const reload = () => accountApi.me().then((m) => { setMe(m); setNick(m.nick_name) }).catch(() => {})
  useEffect(() => { reload() }, [])

  const flash = (m: string) => { setMsg(m); setTimeout(() => setMsg(''), 1800) }

  const saveNick = async () => {
    try {
      const m = await accountApi.updateProfile({ nick_name: nick.trim() })
      setMe(m)
      syncUserCache({ nick_name: m.nick_name })
      flash('昵称已保存')
    } catch (e) { setErr(e instanceof ApiError ? e.message : '保存失败') }
  }

  const saveAvatar = async (emoji: string, bg: string) => {
    const m = await accountApi.updateProfile({ avatar_emoji: emoji, avatar_bg: bg })
    setMe(m)
    syncUserCache({ avatar_emoji: m.avatar_emoji, avatar_bg: m.avatar_bg })
    setAvatarDlg(false)
    flash('头像已更新')
  }

  const doExport = async () => {
    setErr('')
    setExporting(true)
    try {
      await accountApi.exportAll()
      flash('导出完成')
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '导出失败')
    } finally { setExporting(false) }
  }

  return (
    <div className="anim-fade-up space-y-4">
      <div className="rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
        <Row title="头像" value="">
          <div className="flex items-center gap-3">
            <UserAvatar emoji={me?.avatar_emoji} bg={me?.avatar_bg} name={me?.nick_name} size={44} />
            <Button size="sm" variant="outline" data-role="avatar-open" onClick={() => setAvatarDlg(true)}>更换头像</Button>
          </div>
        </Row>
        <Sep />
        <Row title="昵称" value="">
          <div className="flex gap-2">
            <Input value={nick} onChange={(e) => setNick(e.target.value)} className="w-44" />
            <Button size="sm" variant="outline" onClick={saveNick}>保存</Button>
          </div>
        </Row>
        <Sep />
        <Row title="手机号" value={me?.phone ? maskPhone(me.phone) : '未绑定'}>
          <Button size="sm" variant={me?.phone ? 'outline' : 'default'} onClick={() => { setErr(''); setDlg('bind-phone') }}>
            {me?.phone ? '换绑' : '绑定'}
          </Button>
        </Row>
        <Sep />
        <Row title="邮箱" value={me?.email ?? '未绑定'}>
          <Button size="sm" variant={me?.email ? 'outline' : 'default'} onClick={() => { setErr(''); setDlg('bind-email') }}>
            {me?.email ? '换绑' : '绑定'}
          </Button>
        </Row>
        <Sep />
        <Row title="密码" value="">
          <Button size="sm" variant="outline" onClick={() => { setErr(''); setDlg('pwd') }}>修改密码</Button>
        </Row>
        <Sep />
        <Row title="注册时间" value="">
          <span className="font-mono text-[13px] font-semibold text-muted-foreground">{me ? fmtFull(me.create_time) : '—'}</span>
        </Row>
      </div>

      {/* 数据导出 */}
      <div className="rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
        <Row title="导出全部数据" value="">
          <Button size="sm" variant="outline" data-role="export-btn" className="normal-case" disabled={exporting} onClick={doExport}>
            {exporting ? <span className="bk-loader" /> : '导出 .zip'}
          </Button>
        </Row>
        <p className="mt-1 text-[11.5px] font-semibold text-muted-foreground">笔记目录树（.md）+ 待办 + 便签，打包为 .zip 下载</p>
      </div>

      {msg && <p className="anim-pop border-3 border-foreground bg-[var(--neon-green)] px-3 py-1.5 text-sm font-bold">{msg}</p>}
      {err && <p className="anim-pop border-3 border-foreground bg-destructive px-3 py-1.5 text-sm font-bold text-destructive-foreground">{err}</p>}

      <BindDialog kind={dlg} onClose={() => setDlg(null)} onDone={(m) => { setDlg(null); flash(m); reload() }} onErr={setErr} />
      <AvatarDialog
        open={avatarDlg} current={{ emoji: me?.avatar_emoji ?? '', bg: me?.avatar_bg ?? '' }}
        onClose={() => setAvatarDlg(false)}
        onSave={saveAvatar}
        onErr={setErr}
      />
    </div>
  )
}

function fmtFull(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

/* ============ 更换头像（表情 + 底色） ============ */
const AVATAR_EMOJIS = [
  '🐱', '🐶', '🦊', '🐼', '🐨', '🦁', '🐯', '🐸', '🐵', '🦉',
  '🐙', '🦄', '🐳', '🦋', '🌸', '🌿', '🍀', '🌵', '🍁', '🌻',
  '🌊', '⭐', '🌙', '☀️', '🔥', '💧', '🌈', '⚡', '🎯', '🎨',
  '🎧', '🎮', '📚', '✏️', '💡', '🔑', '🧠', '🤖', '👾', '🍩',
]

function AvatarDialog({ open, current, onClose, onSave, onErr }: {
  open: boolean
  current: { emoji: string; bg: string }
  onClose: () => void
  onSave: (emoji: string, bg: string) => Promise<void>
  onErr: (s: string) => void
}) {
  const [emoji, setEmoji] = useState(current.emoji)
  const [bg, setBg] = useState(current.bg)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (open) { setEmoji(current.emoji); setBg(current.bg) } }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  const save = async () => {
    setBusy(true); onErr('')
    try { await onSave(emoji, bg) } catch (e) { onErr(e instanceof ApiError ? e.message : '保存失败') } finally { setBusy(false) }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent data-role="dlg-avatar" className="max-w-[540px]">
        <DialogHeader><DialogTitle>更换头像</DialogTitle></DialogHeader>
        <div className="space-y-4">
          <div className="flex items-center gap-4">
            <UserAvatar emoji={emoji} bg={bg} size={64} />
            <p className="text-[12px] font-semibold leading-relaxed text-muted-foreground">选一个表情和底色即可，无需上传图片。<br />保存后侧栏与设置页同步更新。</p>
          </div>
          <div className="space-y-1.5">
            <Label>表情</Label>
            <div className="flex flex-wrap gap-1.5">
              {AVATAR_EMOJIS.map((e) => (
                <button
                  key={e} type="button" data-emoji={e}
                  className={cn(
                    'flex h-9 w-9 cursor-pointer items-center justify-center rounded-lg border-2 border-foreground text-[19px] transition-shadow',
                    e === emoji ? 'bg-primary shadow-[2px_2px_0px_var(--shadow-color)]' : 'bg-card hover:shadow-[2px_2px_0px_var(--shadow-color)]',
                  )}
                  onClick={() => setEmoji(e)}
                >{e}</button>
              ))}
            </div>
          </div>
          <div className="space-y-1.5">
            <Label>底色</Label>
            <div className="flex gap-2">
              {AVATAR_BG_OPTIONS.map((o) => (
                <button
                  key={o.value} type="button" title={o.label} data-bg={o.value}
                  className={cn(
                    'h-[30px] w-[30px] cursor-pointer rounded-full border-2 border-foreground',
                    o.value === bg && 'ring-2 ring-foreground ring-offset-2 ring-offset-card',
                  )}
                  style={{ background: o.value ? `var(--neon-${o.value})` : 'var(--card)' }}
                  onClick={() => setBg(o.value)}
                />
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" className="mr-auto" onClick={() => { setEmoji(''); setBg('') }}>恢复默认</Button>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button data-role="avatar-save" disabled={busy} onClick={save}>{busy ? <span className="bk-loader" /> : '保存'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({ title, value, children }: { title: string; value?: string; children?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 py-1.5">
      <div>
        <div className="font-extrabold">{title}</div>
        {value && <div className={cn('mt-0.5 font-mono text-[13px]', value === '未绑定' ? 'text-muted-foreground' : '')}>{value}</div>}
      </div>
      <div className="flex items-center gap-2">{children}</div>
    </div>
  )
}
const Sep = () => <div className="border-t-2 border-black/10" />

function BindDialog({ kind, onClose, onDone, onErr }: {
  kind: 'bind-phone' | 'bind-email' | 'pwd' | null
  onClose: () => void; onDone: (msg: string) => void; onErr: (s: string) => void
}) {
  const [value, setValue] = useState('')
  const [pwd, setPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirmPwd, setConfirmPwd] = useState('')
  const [busy, setBusy] = useState(false)
  if (!kind) return null

  async function submit() {
    setBusy(true); onErr('')
    try {
      if (kind === 'pwd') {
        if (newPwd !== confirmPwd) { onErr('两次新密码不一致'); return }
        const r = await accountApi.changePassword(pwd, newPwd)
        setToken(r.token) // 服务端已 +1 会话版本（其他设备全失效）→ 当前设备换用新 token
        onDone('密码已修改')
      } else {
        const r = await accountApi.bind(kind === 'bind-phone' ? 'phone' : 'email', value.trim(), pwd)
        setToken(r.token)
        onDone(kind === 'bind-phone' ? '手机号已更新' : '邮箱已更新')
      }
    } catch (e) {
      onErr(e instanceof ApiError ? e.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  const title = kind === 'pwd' ? '修改密码' : kind === 'bind-phone' ? (value && '换绑手机号' || '绑定手机号') : '绑定邮箱'
  return (
    <Dialog open onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader><DialogTitle>{title}</DialogTitle></DialogHeader>
        <div className="space-y-4">
          {kind !== 'pwd' && (
            <div className="space-y-2">
              <Label>{kind === 'bind-phone' ? '新手机号' : '邮箱'}</Label>
              <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder={kind === 'bind-phone' ? '139…' : 'you@example.com'} />
            </div>
          )}
          {kind === 'pwd' ? (
            <>
              <div className="space-y-2"><Label>当前密码</Label><Input type="password" value={pwd} onChange={(e) => setPwd(e.target.value)} /></div>
              <div className="space-y-2"><Label>新密码</Label><Input type="password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} placeholder="至少 8 位" /></div>
              <div className="space-y-2"><Label>确认新密码</Label><Input type="password" value={confirmPwd} onChange={(e) => setConfirmPwd(e.target.value)} /></div>
            </>
          ) : (
            <div className="space-y-2"><Label>当前密码</Label><Input type="password" value={pwd} onChange={(e) => setPwd(e.target.value)} placeholder="••••••••" /></div>
          )}
          {/* 安全操作提示：服务端会踢掉其他设备（token_version +1） */}
          <div className="flex items-start gap-2 border-3 border-foreground bg-primary px-3 py-2 text-[12px] font-bold">
            <span>⚠️</span>
            <span>{kind === 'pwd' ? '修改后' : '换绑后'}，其他设备上的登录将全部退出，需要重新登录。</span>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? <span className="bk-loader" /> : '确定'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ============ 浏览器通知 ============ */
function PushTab() {
  const [enabled, setEnabled] = useState(false)
  const [vapidOn, setVapidOn] = useState(false)
  const [msg, setMsg] = useState('')

  useEffect(() => {
    pushApi.vapid().then((v) => setVapidOn(v.enabled)).catch(() => setVapidOn(false))
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.getRegistration('/sw.js').then(async (reg) => {
        if (reg) {
          const sub = await reg.pushManager.getSubscription()
          setEnabled(!!sub)
        }
      })
    }
  }, [])

  async function toggle(on: boolean) {
    setMsg('')
    // 会话失效短路：api() 收到 401 会清 token 跳登录页——这里不再写任何 msg，
    // 避免「订阅失败」误导文案在跳转前一瞬闪出（2026-10-03 实测：推送订阅成功后 token 失效，401 被误报）
    if (!getToken()) return
    try {
      if (on) {
        const reg = await navigator.serviceWorker.register('/sw.js')
        await navigator.serviceWorker.ready
        const { public_key } = await pushApi.vapid().then((v) => ({ public_key: v.public_key }))
        const sub = await reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(public_key),
        })
        const j = sub.toJSON()
        await pushApi.subscribe({ endpoint: j.endpoint!, keys: j.keys! as { p256dh: string; auth: string } })
        setEnabled(true)
      } else {
        const reg = await navigator.serviceWorker.getRegistration('/sw.js')
        const sub = await reg?.pushManager.getSubscription()
        if (sub) {
          await pushApi.unsubscribe(sub.endpoint)
          await sub.unsubscribe()
        }
        setEnabled(false)
      }
    } catch (e) {
      // 分类文案：401（会话失效，api.ts 正在跳登录）/ SW 注册失败 / 其余 = 权限或 VAPID
      if (e instanceof ApiError && e.status === 401) return
      if (e instanceof DOMException && /service|worker|certificate|SSL/i.test(e.message)) {
        setMsg('推送服务注册失败（ServiceWorker/证书问题），请刷新页面后重试')
      } else if (e instanceof ApiError) {
        setMsg(e.message)
      } else {
        setMsg('订阅失败：请检查浏览器通知权限（站点需允许通知）后重试')
      }
    }
  }

  return (
    <div className="space-y-4">
      <div className="anim-fade-up rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
        <Row title="桌面推送" value={vapidOn ? undefined : '服务端未配置 VAPID'}>
          <Button size="sm" variant="outline" onClick={async () => { await pushApi.test().catch(() => {}); setMsg('测试已发送（若未收到，检查浏览器通知权限）') }}>发送测试</Button>
          <Switch checked={enabled} onCheckedChange={toggle} disabled={!vapidOn} />
        </Row>
      </div>
      <div className="anim-fade-up rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
        <Row title="今日到期汇总" value="每天 09:00 · 汇总当日到期待办" />
        <Sep />
        <Row title="逾期即时提醒" value="过期未完成 · 逐条推送" />
      </div>
      {msg && <p className="anim-pop border-3 border-foreground bg-primary px-3 py-1.5 text-sm font-bold">{msg}</p>}
    </div>
  )
}

/* ============ 机器人通知 ============ */
function BotTab() {
  const [channels, setChannels] = useState<NotifyChannel[] | null>(null)
  const [addOpen, setAddOpen] = useState(false)
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')

  const reload = () => notifyApi.list().then((r) => setChannels(r.channels)).catch(() => setChannels([]))
  useEffect(() => { reload() }, [])

  return (
    <div className="space-y-4">
      <div className="anim-fade-up flex justify-end">
        <Button onClick={() => { setErr(''); setAddOpen(true) }}>＋ 添加机器人</Button>
      </div>
      {err && <p className="anim-pop border-3 border-foreground bg-destructive px-3 py-1.5 text-sm font-bold text-destructive-foreground">{err}</p>}
      {msg && <p className="anim-pop border-3 border-foreground bg-[var(--neon-green)] px-3 py-1.5 text-sm font-bold">{msg}</p>}
      {(channels ?? []).map((c, i) => (
        <div key={c.id} className="anim-slide-in rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)] transition-transform hover:-translate-y-0.5" style={{ animationDelay: `${i * 0.05}s` }}>
          <div className="flex items-center gap-3">
            <span className={cn('border-3 border-foreground px-2 py-0.5 text-xs font-extrabold',
              c.channel_type === CHANNEL_TYPE.dingtalk ? 'bg-[var(--neon-blue)]' : 'bg-[var(--neon-purple)]')}>
              {c.channel_type === CHANNEL_TYPE.dingtalk ? '钉钉' : '飞书'}
            </span>
            <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{c.webhook_url}</span>
            <Switch checked={!!c.is_enabled} onCheckedChange={async (v) => { await notifyApi.toggle(c.id, v); reload() }} />
          </div>
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <span className={cn('border-2 border-foreground px-1.5 text-xs font-bold',
                c.last_push_result === '成功' ? 'bg-[var(--neon-green)]' : c.last_push_result ? 'bg-destructive text-destructive-foreground' : 'bg-muted text-muted-foreground')}>
                {c.last_push_result || '未测试'}
              </span>
              {c.last_push_time && <span className="text-xs font-semibold text-muted-foreground">{new Date(c.last_push_time).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</span>}
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={async () => {
                const r = await notifyApi.test(c.id)
                setMsg(r.message); setTimeout(() => setMsg(''), 2500); reload()
              }}>测试</Button>
              <Button size="sm" variant="destructive" onClick={async () => { await notifyApi.del(c.id); reload() }}>删除</Button>
            </div>
          </div>
        </div>
      ))}
      {channels?.length === 0 && (
        <div className="anim-fade-up border-3 border-dashed border-foreground/40 py-8 text-center text-sm font-semibold text-muted-foreground">
          还没有机器人，右上添加（钉钉/飞书自定义机器人 Webhook）
        </div>
      )}
      <AddChannelDialog open={addOpen} onClose={() => setAddOpen(false)} onDone={(m) => { setAddOpen(false); setMsg(m); setTimeout(() => setMsg(''), 2500); reload() }} onErr={setErr} />
    </div>
  )
}

function AddChannelDialog({ open, onClose, onDone, onErr }: { open: boolean; onClose: () => void; onDone: (m: string) => void; onErr: (s: string) => void }) {
  const [type, setType] = useState<string>(CHANNEL_TYPE.dingtalk)
  const [url, setUrl] = useState('')
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true); onErr('')
    try {
      const c = await notifyApi.create({ channel_type: type, webhook_url: url.trim(), secret: secret.trim() || undefined })
      onDone(c.last_push_result === '成功' ? '已添加，测试推送成功 ✓' : `已添加（测试：${c.last_push_result}）`)
    } catch (e) {
      onErr(e instanceof ApiError ? e.message : '添加失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader><DialogTitle>添加机器人</DialogTitle></DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>类型</Label>
            <Select value={type} onValueChange={setType}>
              <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value={CHANNEL_TYPE.dingtalk}>钉钉</SelectItem>
                <SelectItem value={CHANNEL_TYPE.feishu}>飞书</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Webhook URL</Label>
            <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://…" />
          </div>
          <div className="space-y-2">
            <Label>加签 Secret（可选）</Label>
            <Input value={secret} onChange={(e) => setSecret(e.target.value)} placeholder="SEC…" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy || !url.trim()}>{busy ? <span className="bk-loader" /> : '添加并测试'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
