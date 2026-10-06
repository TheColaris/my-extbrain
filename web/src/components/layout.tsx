import { useEffect, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { UserAvatar } from '@/components/user-avatar'
import { NoteModal } from '@/components/note-modal'
import { TourGuide } from '@/components/tour-guide'
import { BrandMark } from '@/components/brand-mark'
import { cn } from '@/lib/utils'

// 应用骨架（侧栏形态 + 移动端抽屉化响应式）：
// brand + 分组导航 + 底部用户卡；<lg 侧栏收进抽屉（汉堡开合 + 遮罩），lg+ 常驻。

// tour = 上手引导的聚光目标（tour-guide.tsx 按 [data-tour] 定位）
type NavItem = { label: string; ready: boolean; tour?: string }
const NAV: { group: string; items: NavItem[] }[] = [
  {
    group: '工作台',
    items: [
      { label: '仪表盘', ready: true },
      { label: '待办', ready: true, tour: 'nav-todos' },
      { label: '便签', ready: true, tour: 'nav-memos' },
      { label: '知识库', ready: true, tour: 'nav-notes' },
    ],
  },
  {
    group: '其他',
    items: [
      { label: '回收站', ready: true },
      { label: '设置', ready: true },
      { label: '操作手册', ready: true },
    ],
  },
  {
    group: '开发',
    items: [
      { label: 'API 密钥', ready: true, tour: 'nav-keys' },
      { label: '操作日志', ready: true },
      { label: '通知记录', ready: true },
    ],
  },
]
// 平台组：仅管理员可见（tu_user.is_admin）
const NAV_ADMIN: { group: string; items: NavItem[] } = {
  group: '平台',
  items: [
    { label: '运营看板', ready: true },
    { label: '平台管理', ready: true },
  ],
}

const LABEL_PATH: Record<string, string> = {
  仪表盘: '/dashboard', 待办: '/todos', 便签: '/memos', 知识库: '/notes',
  'API 密钥': '/keys', 操作日志: '/logs', 设置: '/settings',
  回收站: '/trash', 通知记录: '/notify-log', 操作手册: '/guide',
  运营看板: '/admin/ops', 平台管理: '/admin',
}
const PATH_TITLE: Record<string, string> = {
  ...Object.fromEntries(Object.entries(LABEL_PATH).map(([l, p]) => [p, l])),
  '/search': '搜索', // 顶栏全局入口，不占侧栏
}

export function Layout({ user, onLogout }: { user: { id: number; nick_name: string; phone?: string; email?: string; avatar_emoji?: string; avatar_bg?: string; is_admin?: boolean }; onLogout: () => void }) {
  const [drawer, setDrawer] = useState(false)
  const loc = useLocation()
  const navigate = useNavigate()
  const navGroups = user.is_admin ? [...NAV, NAV_ADMIN] : NAV

  // 路由变化时收起抽屉
  useEffect(() => {
    setDrawer(false)
  }, [loc.pathname])

  // ⌘K 全局唤起搜索（/search 页自身聚焦输入框，其余页面跳转过去）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        if (loc.pathname !== '/search') navigate('/search')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [loc.pathname, navigate])

  const active = PATH_TITLE[loc.pathname] ?? ''
  const nav = (label: string) => navigate(LABEL_PATH[label] ?? '/')

  const sidebar = (
    <>
      <div className="flex items-center gap-3 border-b-3 border-foreground p-4">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)]">
          <BrandMark />
        </div>
        <div>
          <div className="text-sm font-extrabold leading-tight">我的外脑</div>
          <div className="font-mono text-[11px] text-muted-foreground">my-extbrain</div>
        </div>
      </div>
      <nav className="flex-1 overflow-y-auto px-3 pt-4">
        {navGroups.map((g) => (
          <div key={g.group} className="mb-4">
            <div className="px-2.5 pb-2 text-[11px] font-bold uppercase tracking-widest text-muted-foreground">{g.group}</div>
            {g.items.map((it) => {
              const activeItem = it.ready && it.label === active
              return (
                <button
                  key={it.label}
                  type="button"
                  data-tour={it.tour}
                  disabled={!it.ready}
                  onClick={() => it.ready && nav(it.label)}
                  className={cn(
                    'mb-1 flex h-9 w-full items-center gap-2.5 rounded-lg border-3 px-2.5 text-sm font-bold transition-all',
                    activeItem ? 'border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)]' : 'border-transparent',
                    !activeItem && it.ready && 'hover:border-foreground hover:bg-accent',
                    !it.ready && 'cursor-not-allowed opacity-35',
                  )}
                >
                  {it.label}
                  {!it.ready && <span className="ml-auto font-mono text-[10px] font-semibold">soon</span>}
                </button>
              )
            })}
          </div>
        ))}
      </nav>
      <div className="border-t-3 border-foreground p-3">
        <div className="flex items-center gap-2.5 rounded-lg border-3 border-foreground bg-background p-2 shadow-[3px_3px_0px_var(--shadow-color)]">
          <UserAvatar emoji={user.avatar_emoji} bg={user.avatar_bg} name={user.nick_name} size={32} />
          <div className="min-w-0 flex-1">
            <div className="truncate text-[13px] font-bold leading-tight">{user.nick_name}</div>
            <div className="truncate font-mono text-[11px] text-muted-foreground">{user.phone ?? user.email}</div>
          </div>
        </div>
        <Button variant="outline" size="sm" className="mt-2 w-full" onClick={() => { onLogout(); navigate('/') }}>
          退出
        </Button>
      </div>
    </>
  )

  return (
    <div className="flex min-h-screen">
      {/* 桌面侧栏（lg+ 常驻） */}
      <aside className="sticky top-0 hidden h-screen w-[250px] shrink-0 flex-col border-r-3 border-foreground bg-card lg:flex">
        {sidebar}
      </aside>

      {/* 移动端抽屉（<lg） */}
      {drawer && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="anim-pop absolute inset-0 bg-foreground/45" onClick={() => setDrawer(false)} />
          <aside className="absolute inset-y-0 left-0 flex w-[262px] max-w-[85vw] flex-col border-r-3 border-foreground bg-card shadow-[6px_0_0_var(--shadow-color)] transition-transform">
            {sidebar}
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex h-[52px] shrink-0 items-center gap-3 border-b-3 border-foreground bg-card px-4 sm:px-6">
          {/* 汉堡（仅移动端） */}
          <button
            type="button"
            aria-label="打开菜单"
            className="flex h-9 w-9 items-center justify-center rounded-lg border-3 border-foreground bg-background shadow-[2px_2px_0px_var(--shadow-color)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-none lg:hidden"
            onClick={() => setDrawer(true)}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" className="h-4 w-4">
              <path d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          </button>
          <span className="text-sm font-bold">{active}</span>
          {/* 全局搜索触发（⌘K）：/search 页由页内大输入框接管，本页不重复出现 */}
          {loc.pathname !== '/search' && (
            <button
              type="button"
              onClick={() => navigate('/search')}
              className="ml-auto flex h-9 cursor-pointer items-center gap-2 rounded-lg border-3 border-foreground bg-background px-3 text-[13px] font-bold text-muted-foreground shadow-[2px_2px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[4px_4px_0px_var(--shadow-color)] max-md:w-9 max-md:justify-center max-md:px-0"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" className="h-4 w-4 shrink-0">
                <circle cx="11" cy="11" r="7" /><path d="m20 20-3.8-3.8" />
              </svg>
              <span className="max-md:hidden">搜索待办、便签、知识库</span>
              <span className="hidden rounded border-2 border-foreground bg-card px-1.5 font-mono text-[10px] font-semibold shadow-[2px_2px_0px_var(--shadow-color)] sm:block">⌘K</span>
            </button>
          )}
        </div>
        {/* 知识库三栏需要横向空间：本页放开 max-width */}
        <div className={cn('mx-auto w-full flex-1 px-4 py-6 sm:px-6 sm:py-8', loc.pathname === '/notes' ? 'max-w-none' : 'max-w-6xl')}>
          <Outlet />
        </div>
      </div>
      {/* 笔记浮窗：?note= 参数驱动，叠加在任意页（/notes 除外） */}
      <NoteModal />
      {/* 上手引导：首登进 /dashboard 自动播一次；?tour=1 重放（移动端导航步自动开抽屉） */}
      <TourGuide uid={user.id} onDrawerChange={setDrawer} />
    </div>
  )
}
