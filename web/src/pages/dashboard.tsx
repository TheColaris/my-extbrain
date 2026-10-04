import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { dashboardApi, type DashboardData } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { cn } from '@/lib/utils'

function greet(): string {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '早上好'
  if (h < 18) return '下午好'
  return '晚上好'
}

function today(): string {
  const d = new Date()
  const week = ['日', '一', '二', '三', '四', '五', '六'][d.getDay()]
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日 周${week}`
}

function fmtDue(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

/* 最近 14 天趋势：双折线（待办黄 / 便签粉）。
   零图表库：SVG 画线（黑描边打底 + 彩线在上，硬边风格），标记点用 HTML 百分比定位——
   SVG preserveAspectRatio="none" 只做等比拉伸，百分比坐标与 viewBox 几何一一对应，标记不受拉伸影响。 */
function TrendChart({ daily }: { daily: NonNullable<DashboardData['daily']> }) {
  const W = 1120
  const H = 170
  const PAD_TOP = 14
  const PAD_BOTTOM = 12
  const plotH = H - PAD_TOP - PAD_BOTTOM
  const n = daily.length || 1
  const max = Math.max(1, ...daily.map((d) => Math.max(d.todos, d.memos)))
  const xAt = (i: number) => ((i + 0.5) / n) * W
  const yAt = (v: number) => PAD_TOP + plotH - (v / max) * plotH
  const lineOf = (pick: (d: NonNullable<DashboardData['daily']>[number]) => number) =>
    daily.map((d, i) => `${xAt(i).toFixed(1)},${yAt(pick(d)).toFixed(1)}`).join(' ')
  const totalTodos = daily.reduce((s, d) => s + d.todos, 0)
  const totalMemos = daily.reduce((s, d) => s + d.memos, 0)

  const series = [
    { key: 'todos' as const, color: 'var(--neon-yellow)', points: lineOf((d) => d.todos), dash: undefined as string | undefined },
    // 便签用虚线：两线在 0 值段完全重合时仍能分辨（实线黄 + 虚线粉）
    { key: 'memos' as const, color: 'var(--neon-pink)', points: lineOf((d) => d.memos), dash: '10 7' },
  ]

  return (
    <section className="anim-fade-up d-2">
      <div className="mb-2 flex items-center gap-2">
        <span className="h-2.5 w-2.5 border-2 border-foreground bg-[var(--neon-yellow)]" />
        <h2 className="text-sm font-extrabold">最近 14 天记录</h2>
        <span className="ml-auto flex items-center gap-3 text-xs font-bold text-muted-foreground">
          <span className="flex items-center gap-1"><i className="inline-block h-2.5 w-2.5 border-2 border-foreground bg-primary not-italic" />待办 {totalTodos}</span>
          <span className="flex items-center gap-1"><i className="inline-block h-2.5 w-2.5 border-2 border-foreground bg-[var(--neon-pink)] not-italic" />便签 {totalMemos}</span>
        </span>
      </div>
      <div className="rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
        <div className="relative h-40">
          <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="h-full w-full">
            {/* 网格：0 / 中 / 满 三条横线 */}
            {[0, 0.5, 1].map((r) => (
              <line
                key={r}
                x1="0"
                x2={W}
                y1={PAD_TOP + plotH * r}
                y2={PAD_TOP + plotH * r}
                className={r === 1 ? 'stroke-foreground/30' : 'stroke-foreground/12'}
                strokeWidth={r === 1 ? 2 : 2}
                strokeDasharray={r === 1 ? undefined : '5 5'}
                vectorEffect="non-scaling-stroke"
              />
            ))}
            {/* 折线：黑描边全部先画（后画的描边会盖住先画的彩线），彩线再统一覆盖在上 */}
            {series.map((s) => (
              <polyline key={'o-' + s.key} points={s.points} fill="none" stroke="var(--shadow-color)" strokeWidth="6" strokeDasharray={s.dash} strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
            ))}
            {series.map((s) => (
              <polyline key={s.key} points={s.points} fill="none" stroke={s.color} strokeWidth="3" strokeDasharray={s.dash} strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
            ))}
          </svg>
          {/* 悬停分栏（原生 title 出明细）+ 非零点标记 */}
          <div className="absolute inset-0 flex">
            {daily.map((d) => {
              const equal = d.todos > 0 && d.todos === d.memos // 两线重合时便签点下移 3px，避免盖住
              return (
                <div
                  key={d.date}
                  className="relative flex-1 hover:bg-foreground/[0.045]"
                  title={`${d.date} · 待办 ${d.todos} · 便签 ${d.memos}`}
                >
                  {d.todos > 0 && (
                    <span
                      className="pointer-events-none absolute h-[9px] w-[9px] -translate-x-1/2 -translate-y-1/2 border-2 border-foreground bg-primary"
                      style={{ top: `${(yAt(d.todos) / H) * 100}%`, left: equal ? 'calc(50% - 4.5px)' : '50%' }}
                    />
                  )}
                  {d.memos > 0 && (
                    <span
                      className="pointer-events-none absolute h-[9px] w-[9px] -translate-x-1/2 -translate-y-1/2 border-2 border-foreground bg-[var(--neon-pink)]"
                      style={{ top: `${(yAt(d.memos) / H) * 100}%`, left: equal ? 'calc(50% + 4.5px)' : '50%' }}
                    />
                  )}
                </div>
              )
            })}
          </div>
        </div>
        <div className="mt-1 flex">
          {daily.map((d, i) => (
            <span key={d.date} className={cn('flex-1 text-center font-mono text-[9px] font-bold text-muted-foreground', i % 2 !== 0 && 'max-sm:hidden')}>{d.date}</span>
          ))}
        </div>
      </div>
    </section>
  )
}

/* 首页底部引导：三步能上手（把「AI 帮你记」核心玩法显式化） */
function GettingStarted() {
  const nav = useNavigate()
  const [copied, setCopied] = useState(false)
  const aiPrompt = [
    '帮我装 ' + location.origin + ' 这个「外脑」（我的个人信息库）Skill，并帮我配上我的 API 密钥：',
    '',
    '1. 安装 CLI 与 Skill：',
    '   curl -fsSL ' + location.origin + '/install.sh | sh',
    '2. 登录授权（会打开浏览器，我点一下「授权」即可）：',
    '   extbrain auth login --server ' + location.origin,
    '3. 装好后：我说「记一下」就用 extbrain todo add；我说「存起来」就写 MD 后 extbrain note push；回答我问题前先 extbrain search 查我的库。',
  ].join('\n')
  return (
    <section className="anim-fade-up d-3 mt-10">
      <div className="mb-3 flex items-center gap-2">
        <span className="h-2.5 w-2.5 border-2 border-foreground bg-[var(--neon-green)]" />
        <h2 className="text-sm font-extrabold">从这里开始</h2>
        <span className="text-xs font-semibold text-muted-foreground">三分钟上手，重点在第 ③ 步</span>
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
          <div className="flex h-7 w-7 items-center justify-center rounded-lg border-3 border-foreground bg-primary text-sm font-extrabold">1</div>
          <div className="mt-2 font-extrabold">记一条东西</div>
          <p className="mt-1 text-xs font-medium leading-relaxed text-muted-foreground">待办管要做的事，便签收一闪而过的想法——跟用备忘录一样简单。</p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={() => nav('/todos')}>去记待办</Button>
            <Button size="sm" variant="outline" onClick={() => nav('/memos')}>去写便签</Button>
          </div>
        </div>
        <div className="rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
          <div className="flex h-7 w-7 items-center justify-center rounded-lg border-3 border-foreground bg-[var(--neon-purple)] text-sm font-extrabold">2</div>
          <div className="mt-2 font-extrabold">攒成知识库</div>
          <p className="mt-1 text-xs font-medium leading-relaxed text-muted-foreground">资料整理成 Markdown 存进来，按目录归档，全文可搜——随时整篇复制带走。</p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" variant="outline" onClick={() => nav('/notes')}>打开知识库</Button>
          </div>
        </div>
        <div className="relative rounded-xl border-3 border-foreground bg-[var(--neon-yellow)] p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
          <span className="absolute -top-2.5 right-3 border-2 border-foreground bg-destructive px-1.5 text-[10px] font-extrabold text-destructive-foreground">核心玩法</span>
          <div className="flex h-7 w-7 items-center justify-center rounded-lg border-3 border-foreground bg-foreground text-sm font-extrabold text-background">3</div>
          <div className="mt-2 font-extrabold">让 AI 替你记</div>
          <p className="mt-1 text-xs font-medium leading-relaxed">签发一把 API 密钥，把下面这段话发给你的 AI（Claude / ChatGPT / ZCode 都行）：</p>
          <div className="mt-2 max-h-24 overflow-auto rounded-lg border-2 border-foreground bg-background p-2 font-mono text-[10px] leading-relaxed">{aiPrompt}</div>
          <div className="mt-2 flex gap-2">
            <Button size="sm" variant="outline" className="bg-background" onClick={async () => { if (await copyText(aiPrompt)) { setCopied(true); setTimeout(() => setCopied(false), 1500) } }}>
              {copied ? '✓ 已复制' : '复制给 AI'}
            </Button>
            <Button size="sm" onClick={() => nav('/keys')}>去签发密钥</Button>
          </div>
        </div>
      </div>
    </section>
  )
}

export function DashboardPage({ nick }: { nick: string }) {
  const [d, setD] = useState<DashboardData | null>(null)
  useEffect(() => {
    dashboardApi.summary().then(setD).catch(() => setD(null))
  }, [])

  const band = [
    { n: d?.todo_count ?? '…', label: '待办' },
    { n: d?.memo_count ?? '…', label: '便签' },
    { n: d?.note_count ?? '…', label: '笔记' },
    { n: d?.due_today ?? '…', label: '今日到期' },
  ]

  return (
    <div className="space-y-8">
      <div className="anim-fade-up py-2">
        <h1 className="text-2xl font-extrabold tracking-tight sm:text-3xl">{greet()}，{nick}</h1>
        <p className="mt-1 text-sm font-semibold text-muted-foreground">{today()}{d && d.due_today > 0 ? ` · ${d.due_today} 项今日到期` : ''}</p>
      </div>

      {/* 数字带 */}
      <div className="anim-fade-up d-1 grid grid-cols-2 overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)] sm:grid-cols-4">
        {band.map((c, i) => (
          <div key={c.label} className={cn('p-5', i > 0 && 'border-l-3 border-foreground', i >= 2 && 'max-sm:border-l-0', i >= 2 && 'max-sm:border-t-3', i === 1 && 'max-sm:border-r-0' && 'sm:border-l-3')}>
            <div className="text-3xl font-extrabold tabular-nums tracking-tight sm:text-4xl">{c.n}</div>
            <div className="mt-1 text-xs font-bold text-muted-foreground">{c.label}</div>
          </div>
        ))}
      </div>

      {d && d.daily && d.daily.some((x) => x.todos + x.memos > 0) && <TrendChart daily={d.daily} />}

      <div className="grid gap-8 lg:grid-cols-2">
        {/* 今日待办 */}
        <section className="anim-fade-up d-2">
          <div className="mb-2 flex items-center gap-2">
            <span className="h-2.5 w-2.5 border-2 border-foreground bg-[var(--neon-purple)]" />
            <h2 className="text-sm font-extrabold">今日待办</h2>
            <Link to="/todos" className="ml-auto text-xs font-bold underline-offset-4 hover:underline">全部 →</Link>
          </div>
          <div className="border-t-2 border-foreground">
            {(d?.today_todos ?? []).map((t) => (
              <div key={t.id} className="flex items-center gap-3 border-b border-black/15 py-2.5">
                <span className="h-3.5 w-3.5 shrink-0 rounded-full border-[2.5px] border-foreground bg-background" />
                <span className="flex-1 truncate text-sm font-semibold">{t.title}</span>
                <span className="shrink-0 border-2 border-foreground bg-destructive px-1.5 py-0.5 font-mono text-xs font-bold text-destructive-foreground">{fmtDue(t.due_time)}</span>
              </div>
            ))}
            {d && d.today_todos.length === 0 && (
              <p className="py-3 text-sm text-muted-foreground">今天没有到期的待办</p>
            )}
          </div>
        </section>

        {/* 最近便签 */}
        <section className="anim-fade-up d-3">
          <div className="mb-2 flex items-center gap-2">
            <span className="h-2.5 w-2.5 border-2 border-foreground bg-[var(--neon-blue)]" />
            <h2 className="text-sm font-extrabold">最近便签</h2>
            <Link to="/memos" className="ml-auto text-xs font-bold underline-offset-4 hover:underline">全部 →</Link>
          </div>
          <div className="border-t-2 border-foreground">
            {(d?.recent_memos ?? []).map((m) => (
              <div key={m.id} className="flex gap-3 border-b border-black/15 py-2.5">
                <span className="w-11 shrink-0 pt-0.5 text-right font-mono text-[11px] font-semibold text-muted-foreground">
                  {new Date(m.create_time).getMonth() + 1}/{new Date(m.create_time).getDate()}
                </span>
                <p className="line-clamp-2 flex-1 text-sm font-medium leading-relaxed">{m.content}</p>
              </div>
            ))}
            {d && d.recent_memos.length === 0 && <p className="py-3 text-sm text-muted-foreground">还没有便签</p>}
          </div>
        </section>
      </div>

      <GettingStarted />
    </div>
  )
}
