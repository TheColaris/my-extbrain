import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { api, keysApi, type APIKeyItem, type LogItem } from '@/lib/api'
import { ACTION, actionLabel } from '@/lib/enums'

const PAGE_SIZE = 20

function fmtTime(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const ACTION_CLASS: Record<string, string> = {
  create: 'bg-primary',
  update: 'bg-primary',
  upsert: 'bg-[var(--neon-yellow)]',
  read: 'bg-[var(--neon-blue)]',
  delete: 'bg-[var(--neon-pink)]',
  failed: 'bg-destructive text-destructive-foreground',
  login: 'bg-[var(--neon-green)]',
  register: 'bg-[var(--neon-green)]',
  query: 'bg-[var(--neon-purple)]',
}

function ActionBadge({ action }: { action: string }) {
  const verb = action.split('.')[1] ?? ''
  const cls = ACTION_CLASS[verb] ?? 'bg-muted'
  return (
    <span className={`whitespace-nowrap border-3 border-foreground px-2 py-0.5 text-xs font-bold ${cls}`}>
      {actionLabel(action)}
    </span>
  )
}

export function LogsPage() {
  const [, setSp] = useSearchParams()
  const [logs, setLogs] = useState<LogItem[] | null>(null)
  const [keys, setKeys] = useState<APIKeyItem[]>([])
  const [keyFilter, setKeyFilter] = useState<string>('all')
  const [actionFilter, setActionFilter] = useState<string>('all')
  const [nextBefore, setNextBefore] = useState(0)

  useEffect(() => {
    keysApi.list().then((r) => setKeys(r.keys)).catch(() => setKeys([]))
  }, [])

  const load = (before: number) => {
    const params = new URLSearchParams({ limit: String(PAGE_SIZE) })
    if (keyFilter === 'web') params.set('key_id', '-1')
    else if (keyFilter !== 'all') params.set('key_id', keyFilter)
    if (actionFilter !== 'all') params.set('action', actionFilter)
    if (before > 0) params.set('before_id', String(before))
    api<{ logs: LogItem[]; next_before_id?: number }>(`/logs?${params}`).then((r) => {
      setLogs(r.logs)
      setNextBefore(r.next_before_id ?? 0)
    })
  }

  // 筛选变化即重查
  useEffect(() => {
    setLogs(null)
    load(0)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [keyFilter, actionFilter])

  return (
    <div className="space-y-6">
      <div className="anim-fade-up flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold tracking-tight">操作日志</h1>
          <p className="mt-0.5 text-xs font-semibold text-muted-foreground">哪个 Key · 哪个 IP · 干了什么 · 保留 90 天</p>
        </div>
      </div>

      {/* 筛选行（data-tour：上手引导第 8 步聚光目标） */}
      <div className="anim-fade-up flex flex-wrap gap-3" data-tour="log-filters">
        <Select value={keyFilter} onValueChange={setKeyFilter}>
          <SelectTrigger className="w-[170px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部 Key</SelectItem>
            <SelectItem value="web">Web 会话</SelectItem>
            {keys.map((k) => (
              <SelectItem key={k.id} value={String(k.id)}>{k.key_name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={actionFilter} onValueChange={setActionFilter}>
          <SelectTrigger className="w-[170px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部操作</SelectItem>
            {Object.values(ACTION).map((a) => (
              <SelectItem key={a} value={a}>{actionLabel(a)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* 日志表 */}
      <div className="anim-fade-up overflow-x-auto rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>时间</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>操作</TableHead>
              <TableHead className="hidden md:table-cell">对象</TableHead>
              <TableHead>IP</TableHead>
              <TableHead className="hidden sm:table-cell">归属地</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="hidden md:table-cell right">耗时</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(logs ?? []).map((l, i) => (
              <TableRow key={l.id} className="anim-slide-in" style={{ animationDelay: `${Math.min(i, 8) * 0.04}s` }}>
                <TableCell className="whitespace-nowrap font-mono text-xs text-muted-foreground">{fmtTime(l.create_time)}</TableCell>
                <TableCell className="whitespace-nowrap text-xs font-bold">
                  {l.api_key_id === 0 ? (
                    <span className="border-2 border-foreground bg-[var(--neon-blue)] px-1.5 py-0.5 text-xs font-bold">Web 面板</span>
                  ) : (
                    keys.find((k) => k.id === l.api_key_id)?.key_name ?? l.key_hint
                  )}
                </TableCell>
                <TableCell><ActionBadge action={l.action} /></TableCell>
                <TableCell className="hidden max-w-48 truncate font-mono text-xs md:table-cell">
                  {l.action.startsWith('note.') && l.target && !l.target.includes('→') && !l.target.includes('#') ? (
                    <button
                      type="button" title={l.target}
                      onClick={() => {
                        // 跳浮窗：带反解出的仓库名（服务端按笔记现归属回填；跨仓库不再 404）
                        const next = new URLSearchParams()
                        next.set('note', l.target.replace(/^\//, ''))
                        if (l.repo_name) next.set('repo', l.repo_name)
                        setSp(next)
                      }}
                      className="max-w-full truncate text-left text-muted-foreground hover:text-foreground hover:underline"
                    >{l.repo_name && <span className="mr-1 inline-flex items-center rounded border-[1.5px] border-foreground bg-[var(--neon-blue)] px-1 text-[10px] font-bold text-foreground" data-role="log-repo">{l.repo_name}</span>}{l.target}</button>
                  ) : (
                    <span className="text-muted-foreground">{l.target}</span>
                  )}
                </TableCell>
                <TableCell className="whitespace-nowrap font-mono text-xs">{l.client_ip}</TableCell>
                <TableCell className="hidden whitespace-nowrap text-xs text-muted-foreground sm:table-cell">{l.ip_region || '—'}</TableCell>
                <TableCell className={`font-mono text-xs font-bold ${l.status_code >= 400 ? 'text-destructive' : ''}`}>{l.status_code}</TableCell>
                <TableCell className="hidden whitespace-nowrap right font-mono text-xs text-muted-foreground md:table-cell">{l.cost_ms}ms</TableCell>
              </TableRow>
            ))}
            {logs?.length === 0 && (
              <TableRow>
                <TableCell colSpan={8} className="py-8 text-center text-sm text-muted-foreground">没有匹配的日志</TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-muted-foreground">{logs ? `${logs.length} 条` : '…'}</span>
        <Button variant="outline" size="sm" disabled={!nextBefore} onClick={() => load(nextBefore)}>
          下一页
        </Button>
      </div>
    </div>
  )
}
