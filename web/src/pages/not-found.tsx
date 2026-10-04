import { useLocation, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { BrandMark } from '@/components/brand-mark'

// 404 页面不存在：
// 登录态访问未知路径时渲染（游客态仍走 403 未登录拦截页，见 not-logged-in.tsx）；
// 出口 = 回主页（默认）/ 返回上一页
export function NotFoundPage() {
  const nav = useNavigate()
  const { pathname, search } = useLocation()
  const blocked = pathname + search

  return (
    <div className="relative flex min-h-screen flex-col overflow-hidden">
      {/* 品牌（左上；点击回主页） */}
      <div className="relative z-10 flex items-center gap-2.5 px-7 py-5">
        <button
          type="button"
          aria-label="回主页"
          onClick={() => nav('/')}
          className="bk-interactive flex h-[30px] w-[30px] cursor-pointer items-center justify-center rounded-[7px] border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)] hover:shadow-[5px_5px_0px_var(--shadow-color)]"
        >
          <BrandMark className="h-[17px] w-[17px]" />
        </button>
        <span className="text-sm font-extrabold">我的外脑</span>
        <span className="font-mono text-[11px] font-semibold text-muted-foreground">my-extbrain</span>
      </div>

      <div className="relative z-10 flex flex-1 items-center justify-center px-6 pb-[72px]">
        <div className="w-full max-w-[560px] text-center">
          {/* 404 数字牌 */}
          <div className="anim-pop flex justify-center gap-3.5">
            <span className="flex h-[94px] w-[84px] -rotate-3 items-center justify-center rounded-xl border-3 border-foreground bg-primary text-[50px] font-extrabold tracking-tight shadow-[6px_6px_0px_var(--shadow-color)] max-md:h-[74px] max-md:w-16 max-md:text-[38px] max-md:shadow-[4px_4px_0px_var(--shadow-color)]">4</span>
            <span className="flex h-[94px] w-[84px] -translate-y-2 rotate-2 items-center justify-center rounded-xl border-3 border-foreground bg-[var(--neon-pink)] text-[50px] font-extrabold tracking-tight shadow-[6px_6px_0px_var(--shadow-color)] max-md:h-[74px] max-md:w-16 max-md:translate-y-0 max-md:text-[38px] max-md:shadow-[4px_4px_0px_var(--shadow-color)]">0</span>
            <span className="flex h-[94px] w-[84px] -rotate-2 items-center justify-center rounded-xl border-3 border-foreground bg-[var(--neon-blue)] text-[50px] font-extrabold tracking-tight shadow-[6px_6px_0px_var(--shadow-color)] max-md:h-[74px] max-md:w-16 max-md:text-[38px] max-md:shadow-[4px_4px_0px_var(--shadow-color)]">4</span>
          </div>

          <h1 className="anim-fade-up d-1 mt-[38px] text-[25px] font-extrabold tracking-tight max-md:mt-7 max-md:text-xl">页面不存在哦</h1>

          {/* 被访问的地址（数据展示；长路径自动换行） */}
          <div className="anim-fade-up d-2 mt-[18px] inline-flex max-w-[min(560px,86vw)] items-center justify-center gap-2 rounded-lg border-2 border-foreground bg-card px-3 py-1.5 shadow-[3px_3px_0px_var(--shadow-color)]">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" stroke-linecap="round" className="h-[13px] w-[13px] shrink-0" aria-hidden>
              <circle cx="12" cy="12" r="9" /><path d="M9.4 9.2a2.7 2.7 0 0 1 5.2 1c0 1.8-2.6 2.2-2.6 3.8" /><circle cx="12" cy="17.6" r="0.4" fill="currentColor" />
            </svg>
            <code className="min-w-0 text-left font-mono text-[12.5px] font-semibold leading-relaxed [overflow-wrap:anywhere]">{blocked}</code>
          </div>

          {/* 出口：主页（默认）｜ 返回上一页 */}
          <div className="anim-fade-up d-3 mt-[34px] flex flex-wrap justify-center gap-3.5">
            <Button size="lg" onClick={() => nav('/')}>回主页</Button>
            <Button size="lg" variant="outline" onClick={() => nav(-1)}>返回上一页</Button>
          </div>
        </div>
      </div>

      {/* 几何装饰（沿用 login/landing/403 的几何语言） */}
      <div className="pointer-events-none absolute -bottom-16 -right-14 z-0 h-[230px] w-[230px] rounded-full border-3 border-foreground bg-[var(--neon-pink)] shadow-[8px_8px_0px_var(--shadow-color)]" />
      <div className="anim-pop d-2 pointer-events-none absolute -left-9 bottom-[76px] z-0 h-24 w-24 -rotate-[14deg] border-3 border-foreground bg-[var(--neon-blue)] shadow-[6px_6px_0px_var(--shadow-color)] max-md:-left-11 max-md:bottom-12" />
      <div className="anim-pop d-3 pointer-events-none absolute right-[132px] top-[86px] z-0 h-[34px] w-[34px] rotate-[18deg] border-3 border-foreground bg-[var(--neon-green)] shadow-[4px_4px_0px_var(--shadow-color)] max-md:right-[18px] max-md:top-[74px]" />
    </div>
  )
}
