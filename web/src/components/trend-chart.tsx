import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

/* 折线图（零图表库，与个人仪表盘同配方）：SVG preserveAspectRatio="none" 拉伸 +
   黑描边全部先画、彩线统一后画 + HTML 百分比定位的方形标记点（不受拉伸影响）。
   交互：悬停出数值（列高亮 + 竖直参考虚线 + tooltip，以可见窗口钳位）；
   长时间轴按每点最小宽度撑开内层 → 面板内左右横滑（默认停在最右=最新，鼠标可拖拽）。 */

export interface TrendSeries {
  name: string
  color: string // CSS 颜色（本项目用 var(--neon-*)）
  dash?: string // 虚线（同图第二/三序列，重合段可分辨）
  values: number[]
}

const W = 1120
const H = 170
const PAD_TOP = 14
const PAD_BOTTOM = 12
const PLOT_H = H - PAD_TOP - PAD_BOTTOM

export function TrendChart({
  dates,
  series,
  minPointWidth = 30,
  height = 150,
}: {
  dates: string[]
  series: TrendSeries[]
  minPointWidth?: number
  height?: number
}) {
  const n = dates.length || 1
  const scrollRef = useRef<HTMLDivElement>(null)
  const colsRef = useRef<HTMLDivElement>(null)
  const tipRef = useRef<HTMLDivElement>(null)
  const guideRef = useRef<HTMLElement>(null)
  const [innerW, setInnerW] = useState(0)
  const [scrollable, setScrollable] = useState(false)
  const [hover, setHover] = useState<number | null>(null)

  const max = Math.max(1, ...series.flatMap((s) => s.values))
  const xAt = (i: number) => ((i + 0.5) / n) * W
  const yAt = (v: number) => PAD_TOP + PLOT_H - (v / max) * PLOT_H

  // 内层宽度：短窗口铺满面板，长窗口按每点最小宽度撑开（横滑）
  const measure = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const w = Math.max(el.clientWidth, n * minPointWidth)
    setInnerW(w)
    setScrollable(w > el.clientWidth + 1)
  }, [n, minPointWidth])

  useLayoutEffect(() => {
    measure()
    const el = scrollRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [measure])

  // 横滑默认停在最右（最新数据）；窗口切换/尺寸变化时重置
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    el.scrollLeft = el.scrollWidth > el.clientWidth + 1 ? el.scrollWidth - el.clientWidth : 0
  }, [innerW, dates.length])

  // 悬停定位：tooltip 以「可见窗口」为界钳位（横滑时不会滑出面板）
  useLayoutEffect(() => {
    const sc = scrollRef.current
    const tip = tipRef.current
    const guide = guideRef.current
    if (!sc || !tip || !guide) return
    if (hover === null) {
      tip.style.display = 'none'
      guide.style.display = 'none'
      return
    }
    const center = ((hover + 0.5) / n) * innerW
    tip.style.display = 'block'
    guide.style.display = 'block'
    guide.style.left = `${center}px`
    const tw = tip.offsetWidth
    const visL = sc.scrollLeft + 4
    const visR = sc.scrollLeft + sc.clientWidth - tw - 4
    tip.style.left = `${visR < visL ? visL : Math.max(visL, Math.min(center - tw / 2, visR))}px`
  }, [hover, innerW, n])

  // 鼠标按住拖拽横滑（触屏/滚轮走原生）
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    let down = false
    let startX = 0
    let startLeft = 0
    const onDown = (e: PointerEvent) => {
      if (e.pointerType !== 'mouse' || e.button !== 0 || !scrollable) return
      down = true
      startX = e.clientX
      startLeft = el.scrollLeft
      el.classList.add('cursor-grabbing')
      try {
        el.setPointerCapture(e.pointerId)
      } catch {
        /* 指针已释放：忽略 */
      }
    }
    const onMove = (e: PointerEvent) => {
      if (down) el.scrollLeft = startLeft - (e.clientX - startX)
    }
    const onUp = () => {
      down = false
      el.classList.remove('cursor-grabbing')
    }
    el.addEventListener('pointerdown', onDown)
    el.addEventListener('pointermove', onMove)
    el.addEventListener('pointerup', onUp)
    el.addEventListener('pointercancel', onUp)
    return () => {
      el.removeEventListener('pointerdown', onDown)
      el.removeEventListener('pointermove', onMove)
      el.removeEventListener('pointerup', onUp)
      el.removeEventListener('pointercancel', onUp)
    }
  }, [scrollable])

  const onOver = (e: React.MouseEvent) => {
    const el = colsRef.current
    const col = (e.target as HTMLElement).closest?.('[data-col]') as HTMLElement | null
    if (!el || !col || col.parentElement !== el) return
    setHover(Number(col.dataset.col))
  }

  const pointsOf = (vals: number[]) =>
    vals.map((v, i) => `${xAt(i).toFixed(1)},${yAt(v).toFixed(1)}`).join(' ')

  // 标记点：同值分组左右错位（两线重合时仍可分辨）
  const markersFor = (i: number) => {
    const groups = new Map<number, number[]>()
    series.forEach((s, si) => {
      const v = s.values[i]
      if (v > 0) groups.set(v, [...(groups.get(v) ?? []), si])
    })
    const out: React.ReactNode[] = []
    groups.forEach((list, v) => {
      list.forEach((si, j) => {
        const dx = list.length > 1 ? (j - (list.length - 1) / 2) * 9 : 0
        out.push(
          <span
            key={`${si}-${v}`}
            className="pointer-events-none absolute h-[9px] w-[9px] -translate-x-1/2 -translate-y-1/2 border-2 border-foreground"
            style={{ left: `calc(50% + ${dx}px)`, top: `${((yAt(v) / H) * 100).toFixed(2)}%`, background: series[si].color }}
          />,
        )
      })
    })
    return out
  }

  const step = n > 20 ? 3 : 1

  return (
    <div
      ref={scrollRef}
      className={cn(
        'overflow-x-auto overflow-y-hidden',
        scrollable && 'cursor-grab',
        // 可见的横滑滚动条（macOS 覆盖式滚动条默认隐藏，不给「可横滑」的可发现性）
        '[&::-webkit-scrollbar]:h-2 [&::-webkit-scrollbar-track]:bg-transparent',
        '[&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:border-2 [&::-webkit-scrollbar-thumb]:border-foreground [&::-webkit-scrollbar-thumb]:bg-[var(--neon-yellow)]',
      )}
      data-role="trend-scroll"
    >
      <div className="relative min-w-full" style={innerW ? { width: innerW } : undefined}>
        <div className="relative" style={{ height }}>
          <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="block h-full w-full">
            {[0, 0.5, 1].map((r) => (
              <line
                key={r}
                x1={0}
                x2={W}
                y1={PAD_TOP + PLOT_H * r}
                y2={PAD_TOP + PLOT_H * r}
                stroke="var(--shadow-color)"
                strokeOpacity={r === 1 ? 0.3 : 0.12}
                strokeWidth={2}
                strokeDasharray={r === 1 ? undefined : '5 5'}
                vectorEffect="non-scaling-stroke"
              />
            ))}
            {/* 黑描边全部先画（后画的描边会盖住先画的彩线），彩线再统一覆盖在上 */}
            {series.map((s, si) => (
              <polyline
                key={`o${si}`}
                points={pointsOf(s.values)}
                fill="none"
                stroke="var(--shadow-color)"
                strokeWidth={6}
                strokeDasharray={s.dash}
                strokeLinejoin="round"
                strokeLinecap="round"
                vectorEffect="non-scaling-stroke"
              />
            ))}
            {series.map((s, si) => (
              <polyline
                key={si}
                points={pointsOf(s.values)}
                fill="none"
                stroke={s.color}
                strokeWidth={3}
                strokeDasharray={s.dash}
                strokeLinejoin="round"
                strokeLinecap="round"
                vectorEffect="non-scaling-stroke"
              />
            ))}
          </svg>
          <div
            ref={colsRef}
            className="absolute inset-0 flex"
            onMouseOver={onOver}
            onMouseLeave={() => setHover(null)}
          >
            {dates.map((d, i) => (
              <div key={`${d}-${i}`} data-col={i} className="relative flex-1 hover:bg-foreground/[0.045]">
                {markersFor(i)}
              </div>
            ))}
          </div>
          <i
            ref={guideRef}
            data-role="trend-guide"
            className="pointer-events-none absolute bottom-0 top-0 hidden border-l-2 border-dashed border-foreground/40"
          />
          <div
            ref={tipRef}
            data-role="trend-tip"
            className="pointer-events-none absolute top-1 z-[3] hidden min-w-[104px] rounded-lg border-2 border-foreground bg-card px-2.5 py-2 shadow-[2px_2px_0px_var(--shadow-color)]"
          >
            {hover !== null && (
              <>
                <b className="mb-1 block font-mono text-[11px] font-extrabold">{dates[hover]}</b>
                {series.map((s) => (
                  <span key={s.name} className="flex items-center gap-1.5 text-[11.5px] font-bold">
                    <i className="inline-block h-[9px] w-[9px] shrink-0 border-2 border-foreground" style={{ background: s.color }} />
                    {s.name}
                    <em className="ml-auto pl-3 font-mono font-extrabold not-italic">{s.values[hover]}</em>
                  </span>
                ))}
              </>
            )}
          </div>
        </div>
        <div className="mt-1 flex">
          {dates.map((d, i) => (
            <span
              key={`${d}-x-${i}`}
              className={cn(
                'flex-1 whitespace-nowrap text-center font-mono text-[9px] font-bold text-muted-foreground',
                (n - 1 - i) % step !== 0 && 'invisible',
              )}
            >
              {d}
            </span>
          ))}
        </div>
      </div>
    </div>
  )
}
