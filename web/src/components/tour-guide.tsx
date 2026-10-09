import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { copyText } from '@/lib/clipboard'
import { aiInstallPrompt } from '@/lib/install-prompt'
import { cn } from '@/lib/utils'

/* 首次上手引导（聚光走查，2026-10-06 首版 5 步；2026-10-09 扩为 9 步跨页，原型 prototype/tour.html）：
   首登进 /dashboard 自动播一次（localStorage extbrain_tour_done:<uid> 记已看，跳过同样记）；
   重放 = /dashboard?tour=1（操作手册页有入口）。9 步跨 4 页（仪表盘 → API 密钥 → 知识库 → 操作日志 → 仪表盘）：
   第 5-8 步为页内步（密钥权限列 / 仓库切换器 / 权限弹窗 / 日志筛选行），步骤带 path 时先导航再轮询等目标渲染；
   页内状态由 URL 参数驱动（?tour=menu 开仓库菜单、?tour=perm 开权限弹窗，见 notes.tsx）。
   每步可跳过、全程 Esc。移动端（<1024）：仅侧栏导航步自动开抽屉；气泡优先贴目标下沿/上沿，兜底落底。
   结构：4 矩形挖洞（.tour-rect）+ 描边环（.tour-ring）+ 气泡（.tour-bubble），样式在 index.css；
   目标元素 = [data-tour]（桌面侧栏与移动抽屉各有一份 DOM，取可见那份）。 */

type Step = {
  /** data-tour 目标值（多个 = 并集高亮）；缺省 = 居中卡 */
  targets?: string[]
  /** 该步所在路由（含查询串）；与当前不同则先导航（跨页步） */
  path?: string
  title: string
  body: string
  /** 收尾步：附安装指令 CTA */
  cta?: boolean
}

const STEPS: Step[] = [
  { title: '60 秒认识你的外脑', body: '你的 AI 负责记，这里负责看和管。跟着点一遍就熟了。' },
  { targets: ['nav-todos', 'nav-memos'], title: '手动记', body: '要做的事进「待办」，一闪而过的想法进「便签」。' },
  { targets: ['nav-notes'], title: '攒资料', body: '资料存成 Markdown 按目录归档，全文可搜，随时整篇带走。' },
  { targets: ['nav-keys'], title: '核心玩法', body: '签发一把 API 密钥，把 AI 接进来，它就能替你记。' },
  { path: '/keys', targets: ['key-scope'], title: '一把钥匙一档权限', body: '签发时给每把钥匙选权限（all / notes / todo）——AI 用哪摊，只开哪摊。' },
  { path: '/notes?tour=menu', targets: ['repo-switcher'], title: '资料分仓库', body: '知识库按仓库分区，工作、学习各管一摊。点这里切换、管理、新建。' },
  { path: '/notes?tour=perm', targets: ['perm-dialog'], title: 'AI 能看什么，你说了算', body: '给仓库或目录配 AI 白名单——只有勾选的 Key 能看见；没配 = 开放。' },
  { path: '/logs', targets: ['log-filters'], title: 'AI 的操作都有记录', body: '哪个 Key、干了什么、对象是谁、来自哪个 IP——想只看某台 AI，按 Key 筛。' },
  { path: '/dashboard', title: '就绪，让 AI 开始记', body: '把安装指令发给你的 AI（Claude Code / ZCode 都行），它就能替你记了。', cta: true },
]

const doneKey = (uid: number) => `extbrain_tour_done:${uid}`
const BUBBLE_W = 316
const HOLE_PAD = 6

type Rect = { left: number; top: number; width: number; height: number }

/** 可见目标元素（桌面侧栏 / 移动抽屉各有一份 DOM，取可见那份） */
function visibleTargets(names?: string[]): HTMLElement[] {
  if (!names?.length) return []
  const out: HTMLElement[] = []
  for (const n of names) {
    for (const el of Array.from(document.querySelectorAll<HTMLElement>(`[data-tour="${n}"]`))) {
      if (el.getClientRects().length > 0 && el.getBoundingClientRect().width > 0) out.push(el)
    }
  }
  return out
}

export function TourGuide({ uid, onDrawerChange }: { uid: number; onDrawerChange: (open: boolean) => void }) {
  const [sp, setSp] = useSearchParams()
  const loc = useLocation()
  const nav = useNavigate()
  const force = sp.get('tour') === '1'
  const [active, setActive] = useState(false)
  const [step, setStep] = useState(0)
  const [copied, setCopied] = useState(false)
  const [hole, setHole] = useState<Rect | null>(null)
  const [bubble, setBubble] = useState({ left: 0, top: 0, width: BUBBLE_W })
  const [arrowTop, setArrowTop] = useState<number | null>(null)
  const [ready, setReady] = useState(false)
  const bubbleRef = useRef<HTMLDivElement>(null)
  const stepRef = useRef(step)
  stepRef.current = step
  const pending = useRef(0)

  /* 布局：量目标 → 挖洞 + 环 + 气泡定位（气泡高度按渲染后实测；居中步洞移出屏外=全遮罩） */
  const layout = useCallback(() => {
    const st = STEPS[stepRef.current]
    const W = window.innerWidth
    const H = window.innerHeight
    const narrowNow = W < 1024
    const els = visibleTargets(st.targets)
    const bh = bubbleRef.current?.offsetHeight ?? 180
    if (!els.length) {
      setHole(null)
      const bw = narrowNow ? W - 24 : BUBBLE_W
      setBubble({ left: narrowNow ? 12 : (W - bw) / 2, top: Math.min(Math.max(12, (H - bh) / 2), H - bh - 12), width: bw })
      setArrowTop(null)
      setReady(true)
      return
    }
    const rects = els.map((e) => e.getBoundingClientRect())
    const l = Math.min(...rects.map((r) => r.left))
    const t = Math.min(...rects.map((r) => r.top))
    const r = Math.max(...rects.map((r) => r.right))
    const b = Math.max(...rects.map((r) => r.bottom))
    setHole({ left: Math.max(0, l - HOLE_PAD), top: Math.max(0, t - HOLE_PAD), width: r - l + HOLE_PAD * 2, height: b - t + HOLE_PAD * 2 })
    if (narrowNow) {
      /* 窄屏气泡：优先贴目标下沿；放不下贴上沿；都不行兜底落底（勿固定落底——高目标如弹窗会盖住底部按钮） */
      const belowTop = b + 12
      const aboveTop = t - bh - 12
      const top = belowTop + bh <= H - 12 ? belowTop : t >= bh + 4 ? Math.max(4, aboveTop) : H - bh - 12
      setBubble({ left: 12, top, width: W - 24 })
      setArrowTop(null)
    } else {
      let left = r + 18
      if (left + BUBBLE_W > W - 12) left = Math.max(12, l - BUBBLE_W - 18)
      const top = Math.min(Math.max(12, t + (b - t) / 2 - bh / 2), H - bh - 12)
      setBubble({ left, top, width: BUBBLE_W })
      setArrowTop(t + (b - t) / 2 - top - 6)
    }
    setReady(true)
  }, [])

  /* 进入某步：跨页步先导航 → 轮询等目标渲染（跨页/数据加载）→ 等 rect 稳定（入场动画/抽屉过渡）→ 量 */
  const place = useCallback((idx: number) => {
    const st = STEPS[idx]
    /* 仅侧栏导航步在窄屏开抽屉；页内步目标在内容区（<lg 知识库树由页内抽屉承接，见 notes.tsx） */
    const needDrawer = window.innerWidth < 1024 && !!st.targets?.some((t) => t.startsWith('nav-'))
    onDrawerChange(needDrawer)
    if (st.path) {
      const cur = window.location.pathname + window.location.search
      if (st.path !== cur) nav(st.path)
    }
    window.clearTimeout(pending.current)
    const startedAt = Date.now()
    let lastKey = ''
    const tick = () => {
      const els = st.targets?.length ? visibleTargets(st.targets) : []
      const found = !st.targets?.length || els.length > 0
      if (!found && Date.now() - startedAt < 2500) {
        pending.current = window.setTimeout(tick, 80)
        return
      }
      /* rect 稳定检测：连续两次采样一致才算落位（页面 fade-up / 抽屉过渡 / 弹窗入场动画期间 rect 在变） */
      const key = els.map((e) => { const r = e.getBoundingClientRect(); return `${r.left},${r.top},${r.width},${r.height}` }).join('|')
      if (key !== lastKey && Date.now() - startedAt < 3000) {
        lastKey = key
        pending.current = window.setTimeout(tick, 90)
        return
      }
      pending.current = window.setTimeout(layout, 40)
    }
    pending.current = window.setTimeout(tick, 30)
  }, [layout, onDrawerChange, nav])

  /* 触发：?tour=1 强制重放；否则首登进 /dashboard 自动播一次 */
  useEffect(() => {
    if (force) { setReady(false); setActive(true); setStep(0); return }
    if (active) return // 引导进行中（如收尾步导航回 /dashboard）不重触发
    if (loc.pathname !== '/dashboard') return
    if (localStorage.getItem(doneKey(uid))) return
    const t = window.setTimeout(() => { setReady(false); setActive(true); setStep(0) }, 450)
    return () => window.clearTimeout(t)
  }, [force, loc.pathname, uid, active])

  useEffect(() => {
    if (!active) return
    setCopied(false)
    place(step)
  }, [active, step, place])

  /* 视口变化：重排（含 <1024 断点跨越） */
  useEffect(() => {
    const onResize = () => { if (active) place(stepRef.current) }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [active, place])

  /* 关闭（跳过/完成/Esc 同路）：记已看 + 收抽屉 + 清 ?tour=*（含页内步的 menu/perm） */
  const close = useCallback(() => {
    setActive(false)
    onDrawerChange(false)
    localStorage.setItem(doneKey(uid), '1')
    if (sp.has('tour')) {
      const next = new URLSearchParams(sp)
      next.delete('tour')
      setSp(next, { replace: true })
    }
  }, [uid, sp, setSp, onDrawerChange])

  useEffect(() => {
    if (!active) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [active, close])

  useEffect(() => () => window.clearTimeout(pending.current), [])

  if (!active) return null
  const st = STEPS[step]
  const last = step === STEPS.length - 1
  const W = window.innerWidth
  const H = window.innerHeight
  const rectStyle = (r: Rect) => ({ left: r.left, top: r.top, width: r.width, height: r.height })
  const zero = { left: 0, top: 0, width: 0, height: 0 }

  return (
    <div role="dialog" aria-modal="true" aria-label="上手引导">
      {/* 4 矩形挖洞；无洞（居中步）时 T 铺满、其余归零 */}
      <div className="tour-rect" style={hole ? { left: 0, top: 0, width: W, height: hole.top } : { left: 0, top: 0, width: W, height: H }} />
      <div className="tour-rect" style={hole ? { left: 0, top: hole.top + hole.height, width: W, height: Math.max(0, H - hole.top - hole.height) } : zero} />
      <div className="tour-rect" style={hole ? { left: 0, top: hole.top, width: hole.left, height: hole.height } : zero} />
      <div className="tour-rect" style={hole ? { left: hole.left + hole.width, top: hole.top, width: Math.max(0, W - hole.left - hole.width), height: hole.height } : zero} />
      <div className={cn('tour-ring', hole && 'on')} style={hole ? rectStyle(hole) : zero} />

      <div
        ref={bubbleRef}
        /* pointer-events-auto：第 7 步权限弹窗（Radix modal）打开时 body 被置 pointer-events:none，气泡需自行恢复可点 */
        className={cn('tour-bubble pointer-events-auto rounded-xl border-3 border-foreground bg-card p-4 shadow-[6px_6px_0px_var(--shadow-color)]', !ready && 'invisible')}
        style={{ left: bubble.left, top: bubble.top, width: bubble.width }}
      >
        {arrowTop !== null && (
          <span className="absolute -left-[7px] h-3 w-3 rotate-45 border-b-2 border-l-2 border-foreground bg-card" style={{ top: arrowTop }} />
        )}
        <div className="flex items-center gap-2.5">
          <span className="shrink-0 rounded-md border-2 border-foreground bg-[var(--neon-yellow)] px-2 py-0.5 font-mono text-[11px] font-extrabold">
            {step + 1} / {STEPS.length}
          </span>
          <span className="text-sm font-extrabold">{st.title}</span>
        </div>
        <p className="mt-2 text-[13px] font-semibold leading-relaxed text-muted-foreground">{st.body}</p>
        {st.cta && (
          <div className="mt-3 border-t-2 border-dashed border-foreground pt-3">
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                onClick={async () => {
                  if (await copyText(aiInstallPrompt())) {
                    setCopied(true)
                    window.setTimeout(() => setCopied(false), 1600)
                  }
                }}
              >
                {copied ? '✓ 已复制' : '复制安装指令给 AI'}
              </Button>
              <Button size="sm" variant="outline" onClick={() => { close(); nav('/keys') }}>去签发密钥</Button>
            </div>
            <p className="mt-2 text-[11px] font-semibold text-muted-foreground">上手引导随时可在「操作手册」页重看</p>
          </div>
        )}
        <div className="mt-3 flex items-center justify-between gap-2.5">
          <span className="flex gap-1.5">
            {STEPS.map((_, i) => (
              <span key={i} className={cn('h-2 w-2 rounded-full border-2 border-foreground', i === step ? 'bg-[var(--neon-yellow)]' : 'bg-card')} />
            ))}
          </span>
          <span className="flex gap-2">
            {!last && <Button size="sm" variant="ghost" onClick={close}>跳过</Button>}
            <Button size="sm" onClick={() => (last ? close() : setStep((s) => s + 1))}>
              {step === 0 ? '开始' : last ? '完成' : '下一步 →'}
            </Button>
          </span>
        </div>
      </div>
    </div>
  )
}
