import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { undoToast } from '@/components/undo-toast'
import { ApiError, notifyLogApi, type PushEvent } from '@/lib/api'
import { cn } from '@/lib/utils'

// 通知记录：
// 按「提醒事件」归并（渠道=扇出 chip）；失败 chip 展开原因+重推；
// 顶部数字带（今天 提醒/投递/失败）；按天分组。

const DAY_MS = 86_400_000

const CHANNEL_LABEL: Record<string, string> = {
  web: '浏览器',
  dingtalk: '钉钉',
  feishu: '飞书',
}

export function NotifyLogPage() {
  const nav = useNavigate()
  const [events, setEvents] = useState<PushEvent[]>([])
  const [stats, setStats] = useState({ events: 0, deliveries: 0, fails: 0 })
  const [result, setResult] = useState<'' | 'fail'>('')
  const [type, setType] = useState<'' | 'due_today' | 'overdue' | 'test'>('')
  const [loading, setLoading] = useState(true)
  const [errMsg, setErrMsg] = useState('')
  const [openKey, setOpenKey] = useState<number>(0)
  const [retrying, setRetrying] = useState<number>(0)

  const load = useCallback(async () => {
    setLoading(true)
    setErrMsg('')
    try {
      const r = await notifyLogApi.list({ result: result || undefined, type: type || undefined, days: 7 })
      setEvents(r.events)
      setStats(r.stats)
    } catch (e) {
      setErrMsg(e instanceof ApiError ? e.message : '加载失败，请稍后重试')
    } finally {
      setLoading(false)
    }
  }, [result, type])
  useEffect(() => { void load() }, [load])

  const doRetry = async (ev: PushEvent, chId: number) => {
    setRetrying(chId)
    try {
      const r = await notifyLogApi.retry(chId)
      await load()
      undoToast(r.ok ? '重推成功' : `重推失败：${r.message}`)
    } catch (e) {
      undoToast(e instanceof ApiError ? e.message : '重推失败')
    } finally {
      setRetrying(0)
      void ev
    }
  }

  // 按天分组（服务器时间即本地时区）
  const groups = useMemo<[string, PushEvent[]][]>(() => {
    const byDay = new Map<string, PushEvent[]>()
    for (const ev of events) {
      const key = dayLabel(ev.create_time)
      if (!byDay.has(key)) byDay.set(key, [])
      byDay.get(key)!.push(ev)
    }
    return [...byDay.entries()] as [string, PushEvent[]][]
  }, [events])

  return (
    <div className="mx-auto w-full max-w-6xl px-6 py-8">
      <div className="page-head-wrap">
        <h1 className="text-[20px] font-extrabold tracking-tight">通知记录</h1>
        <p className="mt-0.5 text-[12px] font-medium text-muted-foreground">一条提醒 · 多渠道投递 · 成败与原因 · 保留 90 天</p>
      </div>

      {/* 数字带（只放可数的量） */}
      <div className="mt-5 grid grid-cols-3 overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)]">
        <StatCell num={stats.events} label="今天提醒" />
        <StatCell num={stats.deliveries} label="今天投递" divider />
        <StatCell num={stats.fails} label="今天失败" divider bad={stats.fails > 0} />
      </div>

      {/* 筛选 */}
      <div className="mt-4 flex flex-wrap gap-2">
        <FilterSelect value={result} onChange={(v) => setResult(v as typeof result)} options={[['', '全部结果'], ['fail', '仅含失败']]} />
        <FilterSelect value={type} onChange={(v) => setType(v as typeof type)} options={[['', '全部类型'], ['due_today', '今日到期'], ['overdue', '逾期提醒'], ['test', '测试']]} />
        <span className="inline-flex h-9 items-center rounded-lg border-3 border-foreground bg-card px-3 text-[12.5px] font-bold text-muted-foreground">近 7 天</span>
      </div>

      {/* 列表 */}
      {loading ? (
        <div className="mt-4">
          {[0, 1].map((i) => (
            <div key={i} className="border-b-2 border-foreground/10 py-4">
              <div className="h-3 w-[50%] rounded bg-foreground/10" />
              <div className="mt-2.5 h-3 w-[25%] rounded bg-foreground/10" />
            </div>
          ))}
        </div>
      ) : errMsg && events.length === 0 ? (
        <div className="mt-6 rounded-xl border-3 border-foreground border-l-[7px] border-l-[var(--neon-red)] bg-card p-5 text-[13px] font-bold shadow-[4px_4px_0px_var(--shadow-color)]">{errMsg}</div>
      ) : events.length === 0 ? (
        <div className="mt-6 rounded-xl border-[3px] border-dashed border-foreground bg-white/45 p-11 text-center">
          <div className="text-[15px] font-extrabold">还没有推送记录</div>
          <div className="mt-1.5 text-[13px] font-semibold text-muted-foreground">到期扫描 09:00 · 逾期即时 · 测试按钮都会留痕</div>
        </div>
      ) : (
        groups.map(([day, evts]) => (
          <div key={day}>
            <div className="mb-1 mt-6 text-[12px] font-extrabold uppercase tracking-wider text-muted-foreground">{day}</div>
            {evts.map((ev) => {
              const failChannels = ev.channels.filter((c) => c.status === 'fail')
              return (
                <div key={ev.event_key + ev.create_time} className="flex items-start gap-3 border-b-2 border-foreground/10 py-3.5">
                  <span className="mt-1 w-11 shrink-0 whitespace-nowrap text-right font-mono text-[11px] font-semibold text-muted-foreground max-sm:w-9">{fmtTime(ev.create_time)}</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2 text-[13.5px] font-bold leading-relaxed [overflow-wrap:anywhere]">
                      <TypeBadge type={ev.event_type} />
                      {ev.title}
                    </div>
                    {ev.body && <p className="mt-0.5 text-[12.5px] font-medium leading-relaxed text-muted-foreground [overflow-wrap:anywhere]">{ev.body}</p>}
                    {/* 渠道扇出 */}
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      {ev.channels.map((c) => {
                        const ok = c.status === 'ok'
                        if (ok) {
                          return (
                            <span key={c.id} className={cn('inline-flex h-[22px] items-center gap-1 rounded-md border-2 border-foreground px-2 text-[11px] font-bold', channelBg(c.channel_type))}>
                              <span className="font-extrabold text-foreground">✓</span>{CHANNEL_LABEL[c.channel_type]} 已推送
                            </span>
                          )
                        }
                        return (
                          <button
                            key={c.id} type="button"
                            onClick={() => setOpenKey(openKey === c.id ? 0 : c.id)}
                            className={cn('inline-flex h-[22px] cursor-pointer items-center gap-1 rounded-md border-2 border-foreground px-2 text-[11px] font-bold transition-shadow duration-300 hover:shadow-[2px_2px_0px_var(--shadow-color)]', openKey === c.id ? 'bg-[var(--neon-pink)]' : 'bg-[var(--neon-red)]')}
                          >
                            <span className="font-extrabold text-foreground">✗</span>{CHANNEL_LABEL[c.channel_type]} 失败
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" className={cn('h-2.5 w-2.5 transition-transform', openKey === c.id && 'rotate-180')}><path d="m6 9 6 6 6-6" /></svg>
                          </button>
                        )
                      })}
                    </div>
                    {/* 失败展开：原因+耗时+重试+重推 */}
                    {ev.channels.filter((c) => c.status === 'fail').map((c) => (
                      <div key={c.id} className={cn('mt-2 rounded-lg border-[3px] border-dashed border-foreground bg-card p-3', openKey !== c.id && 'hidden')}>
                        <div className="text-[12.5px] font-semibold leading-relaxed text-[#c23434] [overflow-wrap:anywhere]">{c.error || '未知原因'}</div>
                        <div className="mt-1.5 text-[11.5px] font-semibold text-muted-foreground">
                          {c.retry_count > 0 && <span>重试 {c.retry_count} 次 · </span>}耗时 {c.cost_ms}ms · {fmtDayTime(c.create_time)}
                        </div>
                        <div className="mt-2.5 flex flex-wrap gap-2">
                          <Button size="sm" disabled={retrying === c.id} onClick={() => doRetry(ev, c.id)}>
                            {retrying === c.id ? '推送中…' : `重推到${CHANNEL_LABEL[c.channel_type]}`}
                          </Button>
                          <Button size="sm" variant="ghost" className="border-2 border-transparent hover:border-foreground" onClick={() => nav('/settings')}>
                            {c.channel_type === 'web' ? '重新订阅' : '渠道设置'}
                          </Button>
                        </div>
                      </div>
                    ))}
                    {failChannels.length === 0 && null}
                  </div>
                </div>
              )
            })}
          </div>
        ))
      )}

      {!loading && events.length > 0 && (
        <div className="mt-4 text-[12px] font-semibold text-muted-foreground">
          {events.length} 条提醒 · {events.reduce((n, e2) => n + e2.channels.length, 0)} 次投递 · {events.reduce((n, e2) => n + e2.channels.filter((c) => c.status === 'fail').length, 0)} 失败 · 近 7 天
        </div>
      )}
    </div>
  )
}

function StatCell({ num, label, divider, bad }: { num: number; label: string; divider?: boolean; bad?: boolean }) {
  return (
    <div className={cn('px-4 pb-4 pt-3.5', divider && 'border-l-2 border-foreground')}>
      <div className={cn('text-[30px] font-extrabold leading-none tracking-tight', bad && 'text-[#c23434]')}>{num}</div>
      <div className="mt-1 text-[12px] font-semibold text-muted-foreground">{label}</div>
    </div>
  )
}

function TypeBadge({ type }: { type: string }) {
  const cls = 'inline-flex h-[19px] shrink-0 items-center rounded-md border-2 border-foreground px-1.5 text-[11px] font-extrabold'
  if (type === 'due_today') return <span className={cn(cls, 'bg-primary')}>今日到期</span>
  if (type === 'overdue') return <span className={cn(cls, 'bg-[var(--neon-red)]')}>逾期提醒</span>
  return <span className={cn(cls, 'bg-card')}>测试</span>
}

function channelBg(ch: string): string {
  if (ch === 'web') return 'bg-[var(--neon-blue)]'
  if (ch === 'dingtalk') return 'bg-primary'
  return 'bg-[var(--neon-purple)]'
}

function FilterSelect({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: [string, string][] }) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="h-9 cursor-pointer rounded-lg border-3 border-foreground bg-card px-2.5 text-[12.5px] font-bold outline-none"
    >
      {options.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
    </select>
  )
}

function dayLabel(iso: string): string {
  const d = new Date(iso)
  const today = new Date()
  if (d.toDateString() === today.toDateString()) return '今天'
  const yesterday = new Date(today.getTime() - DAY_MS)
  if (d.toDateString() === yesterday.toDateString()) return '昨天'
  return `${d.getMonth() + 1}-${String(d.getDate()).padStart(2, '0')}`
}
function fmtTime(iso: string): string {
  const d = new Date(iso)
  const today = new Date()
  if (d.toDateString() === today.toDateString()) {
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
  }
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
function fmtDayTime(iso: string): string {
  const d = new Date(iso)
  return `${d.getMonth() + 1}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
}
