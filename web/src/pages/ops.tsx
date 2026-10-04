import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ApiError, adminApi, type OpsDay, type OpsSummary } from '@/lib/api'
import { actionLabel } from '@/lib/enums'
import { TrendChart, type TrendSeries } from '@/components/trend-chart'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'

// 运营看板（仅管理员）：
// 数字带 2×4（主数字只放可数量，比率降副行）+ 4 折线（悬停出数值；30 天面板内横滑）
// + 动作 Top / 异常 + 活跃用户 Top 10。
// 口径：行为类=审计事件（Web 读类成功不记审计，活跃偏保守）；
// 分桶按服务器本地时区；窗口受 LOG_RETENTION_DAYS（默认 90 天）约束。

const DAY_OPTIONS: Array<14 | 30> = [14, 30]

// 错误码文案（看板展示层；未收录的码回显「状态 N」）
const STATUS_LABEL: Record<number, string> = {
  400: '参数错误',
  401: '认证失败',
  403: '无权限',
  404: '目标不存在',
  409: '冲突',
  429: '请求过频',
  500: '服务错误',
  502: '上游异常',
  503: '暂不可用',
}

const AVATAR_COLORS = ['var(--neon-yellow)', 'var(--neon-pink)', 'var(--neon-blue)', 'var(--neon-green)', 'var(--neon-purple)', 'var(--neon-orange)']

type ChartDef = {
  title: string
  accent: string
  series: Array<{ name: string; color: string; dash?: string; pick: (d: OpsDay) => number }>
}

const CHARTS: ChartDef[] = [
  {
    title: '用户与活跃',
    accent: 'var(--neon-yellow)',
    series: [
      { name: '新增注册', color: 'var(--neon-yellow)', pick: (d) => d.registers },
      { name: '活跃人次', color: 'var(--neon-pink)', dash: '10 7', pick: (d) => d.actives },
    ],
  },
  {
    title: '调用量',
    accent: 'var(--neon-purple)',
    series: [
      { name: 'AI', color: 'var(--neon-purple)', pick: (d) => d.calls_ai },
      { name: 'Web', color: 'var(--neon-blue)', dash: '10 7', pick: (d) => d.calls_web },
    ],
  },
  {
    title: '内容产出',
    accent: 'var(--neon-blue)',
    series: [
      { name: '笔记', color: 'var(--neon-yellow)', pick: (d) => d.notes },
      { name: '待办', color: 'var(--neon-blue)', pick: (d) => d.todos },
      { name: '便签', color: 'var(--neon-pink)', dash: '10 7', pick: (d) => d.memos },
    ],
  },
  {
    title: '错误',
    accent: 'var(--neon-red)',
    series: [
      { name: '4xx', color: 'var(--neon-purple)', pick: (d) => d.err4xx },
      { name: '5xx', color: 'var(--neon-red)', dash: '10 7', pick: (d) => d.err5xx },
    ],
  },
]

export function OpsPage() {
  const [days, setDays] = useState<14 | 30>(14)
  const [data, setData] = useState<OpsSummary | null>(null)
  const [loading, setLoading] = useState(true) // 首屏骨架
  const [busy, setBusy] = useState(false) // 切换窗口/刷新
  const [err, setErr] = useState('')

  const load = useCallback(async (d: 14 | 30, initial = false) => {
    if (!initial) setBusy(true)
    try {
      const out = await adminApi.ops(d)
      setData(out)
      setErr('')
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : '加载失败'
      if (initial) setErr(msg)
      else toast.error(msg)
    } finally {
      setLoading(false)
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    load(days, true)
  }, [days, load])

  if (loading && !data) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-extrabold tracking-tight">运营看板</h1>
        <div className="h-[200px] animate-pulse rounded-xl border-3 border-foreground/10 bg-card" />
        <div className="grid gap-6 lg:grid-cols-2">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="h-[220px] animate-pulse rounded-xl border-3 border-foreground/10 bg-card" />
          ))}
        </div>
      </div>
    )
  }
  if (!data) {
    return (
      <div className="space-y-6">
        <h1 className="text-xl font-extrabold tracking-tight">运营看板</h1>
        <div className="rounded-xl border-3 border-foreground border-l-[7px] border-l-[var(--neon-red)] bg-card p-6 text-sm font-bold shadow-[4px_4px_0px_var(--shadow-color)]">
          {err || '加载失败'}
        </div>
      </div>
    )
  }

  const ov = data.overview
  const dates = data.daily.map((d) => d.date)
  const pushFail = Math.max(0, ov.push_7d_total - ov.push_7d_ok)
  const band = [
    { value: ov.users_total, label: '总用户', sub: `今日 +${ov.users_today}` },
    { value: ov.active_today, label: '今日活跃', sub: `7 日 ${ov.active_7d}` },
    { value: ov.calls_today, label: '今日调用', sub: `7 日均 ${Math.round(ov.calls_7d_avg)}` },
    { value: ov.errors_today, label: '今日错误', sub: `占 ${(ov.error_rate_today * 100).toFixed(1)}%` },
    { value: ov.notes_total, label: '笔记', sub: `今日 +${ov.notes_today}` },
    { value: ov.todos_total, label: '待办', sub: '未完成' },
    { value: ov.memos_total, label: '便签', sub: `今日 +${ov.memos_today}` },
    { value: pushFail, label: '推送失败', sub: `7 日 ${ov.push_7d_ok}/${ov.push_7d_total}` },
  ]
  const maxAction = Math.max(1, ...data.top_actions.map((a) => a.count))

  return (
    <div className="space-y-7">
      {/* ===== 页头 ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-extrabold tracking-tight">运营看板</h1>
        <div className="flex items-center gap-2.5">
          <div className="inline-flex overflow-hidden rounded-lg border-3 border-foreground bg-card shadow-[2px_2px_0px_var(--shadow-color)]">
            {DAY_OPTIONS.map((d) => (
              <button
                key={d}
                type="button"
                data-role="ops-days"
                onClick={() => setDays(d)}
                className={cn(
                  'border-r-2 border-foreground px-4 py-1.5 text-[13px] font-bold transition-colors last:border-r-0',
                  days === d ? 'bg-primary' : 'hover:bg-accent',
                )}
              >
                {d} 天
              </button>
            ))}
          </div>
          <Button size="sm" variant="outline" data-role="ops-refresh" disabled={busy} onClick={() => load(days)}>
            {busy ? '刷新中…' : '刷新'}
          </Button>
        </div>
      </div>

      {/* ===== 数字带 2×4（主数字只放可数量）===== */}
      <div
        data-role="ops-band"
        className="grid grid-cols-4 overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)] max-md:grid-cols-2"
      >
        {band.map((c, i) => (
          <div
            key={c.label}
            className={cn(
              'border-l-2 border-t-2 border-foreground px-4 py-3.5',
              i % 2 === 0 && 'border-l-0',
              i < 2 && 'border-t-0',
              i % 4 === 0 && 'md:border-l-0',
              i < 4 && 'md:border-t-0',
            )}
          >
            <div className="text-3xl font-extrabold leading-none tracking-tight tabular-nums">{c.value}</div>
            <div className="mt-2 flex items-baseline justify-between gap-2">
              <b className="text-xs font-bold text-muted-foreground">{c.label}</b>
              <i className="whitespace-nowrap font-mono text-[11px] font-bold not-italic text-muted-foreground/80">{c.sub}</i>
            </div>
          </div>
        ))}
      </div>

      {/* ===== 折线图 2×2（悬停出数值；30 天面板内横滑）===== */}
      <div className="grid gap-6 lg:grid-cols-2">
        {CHARTS.map((c) => {
          const series: TrendSeries[] = c.series.map((s) => ({
            name: s.name,
            color: s.color,
            dash: s.dash,
            values: data.daily.map(s.pick),
          }))
          return (
            // min-w-0 必需：否则 x 轴 nowrap 标签的 min-content 会把 grid 列撑宽、整页横向溢出
            <section key={c.title} className="min-w-0" data-role="ops-chart">
              <div className="mb-2 flex items-center gap-2">
                <span className="h-2.5 w-2.5 shrink-0 rounded-[3px] border-2 border-foreground" style={{ background: c.accent }} />
                <h2 className="text-[13px] font-extrabold">{c.title}</h2>
                <span className="ml-auto flex items-center gap-3 font-mono text-[11px] font-bold text-muted-foreground">
                  {series.map((s) => (
                    <span key={s.name} className="flex items-center gap-1.5">
                      <i className="inline-block h-2.5 w-2.5 border-2 border-foreground" style={{ background: s.color }} />
                      {s.name} {s.values.reduce((a, b) => a + b, 0)}
                    </span>
                  ))}
                </span>
              </div>
              <div className="rounded-xl border-3 border-foreground bg-card px-4 pb-2.5 pt-3.5 shadow-[4px_4px_0px_var(--shadow-color)]">
                <TrendChart dates={dates} series={series} />
              </div>
            </section>
          )
        })}
      </div>

      {/* ===== 动作 Top / 异常 ===== */}
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="min-w-0 rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
          <div className="mb-3 flex items-center gap-2 text-sm font-extrabold">
            <span className="h-2.5 w-2.5 rounded-[3px] border-2 border-foreground bg-[var(--neon-purple)]" />
            动作 Top
          </div>
          {data.top_actions.length === 0 ? (
            <div className="py-6 text-center text-[13px] font-semibold text-muted-foreground">暂无调用</div>
          ) : (
            <div className="space-y-1">
              {data.top_actions.map((a) => (
                <div key={a.action} className="grid grid-cols-[128px_1fr_46px] items-center gap-2.5 py-1">
                  <span className="truncate text-[12.5px] font-bold">{actionLabel(a.action)}</span>
                  <div className="h-3.5 overflow-hidden rounded border-2 border-foreground bg-background">
                    <i
                      className="block h-full border-r-2 border-foreground bg-[var(--neon-purple)]"
                      style={{ width: `${Math.max(4, Math.round((a.count / maxAction) * 100))}%` }}
                    />
                  </div>
                  <span className="text-right font-mono text-xs font-bold">{a.count}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="min-w-0 rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]">
          <div className="mb-3 flex items-center gap-2 text-sm font-extrabold">
            <span className="h-2.5 w-2.5 rounded-[3px] border-2 border-foreground bg-[var(--neon-red)]" />
            异常
          </div>
          {data.top_errors.length === 0 ? (
            <div className="py-6 text-center text-[13px] font-semibold text-muted-foreground">暂无错误</div>
          ) : (
            <div>
              {data.top_errors.map((e) => (
                <div key={e.status} className="flex items-center gap-3 border-b border-foreground/15 py-2.5 last:border-b-0">
                  <span className="inline-flex h-[22px] min-w-[46px] items-center justify-center rounded-md border-2 border-foreground bg-[var(--neon-red)] px-2 font-mono text-[11px] font-bold">
                    {e.status}
                  </span>
                  <span className="flex-1 truncate text-[13px] font-semibold">{STATUS_LABEL[e.status] ?? `状态 ${e.status}`}</span>
                  <span className="font-mono text-[13px] font-bold">{e.count}</span>
                </div>
              ))}
            </div>
          )}
          <div className="my-3.5 border-t-2 border-foreground" />
          <div className="flex items-center justify-between">
            <span className="text-[13px] font-extrabold">推送 · 7 日</span>
            <span className="flex items-center gap-2">
              <span className="inline-flex h-[22px] items-center rounded-md border-2 border-foreground bg-[var(--neon-green)] px-2 text-[11px] font-bold">
                成功 {ov.push_7d_ok}
              </span>
              <span className="inline-flex h-[22px] items-center rounded-md border-2 border-foreground bg-[var(--neon-red)] px-2 text-[11px] font-bold">
                失败 {pushFail}
              </span>
            </span>
          </div>
        </div>
      </div>

      {/* ===== 活跃用户 Top 10 ===== */}
      <div>
        <div className="mb-3 flex items-center gap-2 text-sm font-extrabold">
          <span className="h-2.5 w-2.5 rounded-[3px] border-2 border-foreground bg-[var(--neon-purple)]" />
          活跃用户 Top 10
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[44px]">#</TableHead>
              <TableHead>用户</TableHead>
              <TableHead>调用量</TableHead>
              <TableHead className="hidden sm:table-cell">笔记</TableHead>
              <TableHead className="hidden sm:table-cell">待办</TableHead>
              <TableHead className="hidden sm:table-cell">便签</TableHead>
              <TableHead>最近活跃</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.top_users.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-6 text-center text-[13px] font-semibold text-muted-foreground">
                  暂无数据
                </TableCell>
              </TableRow>
            ) : (
              data.top_users.map((u, i) => (
                <TableRow key={u.user_id} data-role="ops-user">
                  <TableCell className="font-mono text-xs">{i + 1}</TableCell>
                  <TableCell>
                    <span className="flex items-center gap-2">
                      <span
                        className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full border-2 border-foreground text-[11px] font-extrabold"
                        style={{ background: AVATAR_COLORS[i % AVATAR_COLORS.length] }}
                      >
                        {u.nick_name.slice(0, 1)}
                      </span>
                      <span className="truncate text-[13px] font-bold">{u.nick_name}</span>
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-[13px]">{u.calls}</TableCell>
                  <TableCell className="hidden font-mono text-[13px] sm:table-cell">{u.notes}</TableCell>
                  <TableCell className="hidden font-mono text-[13px] sm:table-cell">{u.todos}</TableCell>
                  <TableCell className="hidden font-mono text-[13px] sm:table-cell">{u.memos}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{relTime(u.last_active)}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

function relTime(iso: string): string {
  const t = new Date(iso)
  if (Number.isNaN(t.getTime())) return ''
  const now = new Date()
  const diff = Math.max(0, now.getTime() - t.getTime())
  const min = Math.floor(diff / 60000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hhmm = `${String(t.getHours()).padStart(2, '0')}:${String(t.getMinutes()).padStart(2, '0')}`
  const dayStart = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const dayDiff = Math.round((dayStart(now) - dayStart(t)) / 86400000)
  if (dayDiff === 0) return `${hhmm}`
  if (dayDiff === 1) return `昨天 ${hhmm}`
  return `${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`
}
