import { useEffect, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

/* 空态动画演示（2026-10-06 指引动画 B 批；原型 prototype/assets/empty-demo.js）：
   打字机打出「你对 AI 说」的一句话 → ↓ → 结果卡飞入 → 停顿后循环。
   「跳过」= 立即落终态并停循环，且按浏览器记住（跳过后不再自动播，直接静态终态）。
   用法：<EmptyDemo say="..."><结果卡/></EmptyDemo>，放空态容器内。 */

const SKIP_KEY = 'extbrain_empty_demo_off'

export function EmptyDemo({ say, children }: { say: string; children: ReactNode }) {
  const [skipped, setSkipped] = useState(() => localStorage.getItem(SKIP_KEY) === '1')
  const [reduced] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches)
  const staticMode = skipped || reduced
  const [typed, setTyped] = useState(staticMode ? say.length : 0)
  const [arrow, setArrow] = useState(staticMode)
  const [shown, setShown] = useState(staticMode)

  useEffect(() => {
    if (staticMode) return
    const timers: number[] = []
    let typer = 0
    const T = (ms: number, fn: () => void) => { timers.push(window.setTimeout(fn, ms)) }
    const cycle = () => {
      setTyped(0)
      setArrow(false)
      setShown(false)
      let i = 0
      T(420, () => {
        typer = window.setInterval(() => {
          i++
          setTyped(i)
          if (i >= say.length) {
            window.clearInterval(typer)
            T(180, () => {
              setArrow(true)
              T(340, () => {
                setShown(true)
                T(4600, cycle)
              })
            })
          }
        }, 36)
      })
    }
    cycle()
    return () => {
      timers.forEach(window.clearTimeout)
      window.clearInterval(typer)
    }
  }, [staticMode, say])

  const skip = () => {
    localStorage.setItem(SKIP_KEY, '1')
    setSkipped(true)
    setTyped(say.length)
    setArrow(true)
    setShown(true)
  }

  return (
    <div className="mx-auto mt-2.5 max-w-[460px]">
      <div className="flex min-h-[41px] items-center gap-2 rounded-lg border-2 border-foreground bg-[#111] px-3.5 py-2.5 text-left font-mono text-xs text-[#e4e4e7] shadow-[3px_3px_0px_var(--shadow-color)]">
        <span className="shrink-0 font-bold text-[var(--neon-yellow)]">你对 AI 说</span>
        <span className="min-w-0 [overflow-wrap:anywhere]">{say.slice(0, typed)}</span>
        <span className="demo-cursor h-[13px] w-[7px] shrink-0 bg-[var(--neon-green)]" />
        {!skipped && (
          <button
            type="button"
            onClick={skip}
            className="ml-auto shrink-0 cursor-pointer text-[11px] font-bold text-[#8a857d] transition-colors hover:text-[var(--neon-green)] hover:underline"
          >
            跳过
          </button>
        )}
      </div>
      <div className={cn('my-[7px] text-center text-[15px] font-extrabold leading-none transition-all duration-[250ms]', arrow ? 'opacity-100' : '-translate-y-[3px] opacity-0')}>
        ↓
      </div>
      <div className={cn('transition-all duration-300 ease-[cubic-bezier(.2,.8,.3,1)]', shown ? 'opacity-100' : 'translate-y-2.5 scale-[.97] opacity-0')}>
        {children}
      </div>
    </div>
  )
}
