import { cn } from '@/lib/utils'

// 用户头像（表情 + 底色，零上传）：emoji 为空时回退昵称首字母；
// 底色 = AVATAR_BG 枚举值 → --neon-<name> 主题变量（空/未知回退默认蓝）。
export function UserAvatar({ emoji, bg, name, size = 32, className }: {
  emoji?: string
  bg?: string
  name?: string
  size?: number
  className?: string
}) {
  const color = bg ? `var(--neon-${bg})` : 'var(--neon-blue)'
  return (
    <div
      data-role="user-avatar"
      className={cn('flex shrink-0 items-center justify-center rounded-full border-3 border-foreground font-extrabold', className)}
      style={{ width: size, height: size, background: color, fontSize: emoji ? size * 0.5 : size * 0.42 }}
    >
      {emoji || (name || '?').slice(0, 1).toUpperCase()}
    </div>
  )
}

/** 同步用户缓存（localStorage）+ 通知 App 刷新侧栏头像 */
export function syncUserCache(patch: { nick_name?: string; avatar_emoji?: string; avatar_bg?: string }) {
  try {
    const cur = JSON.parse(localStorage.getItem('extbrain_user') ?? '{}')
    localStorage.setItem('extbrain_user', JSON.stringify({ ...cur, ...patch }))
    window.dispatchEvent(new Event('extbrain:user-changed'))
  } catch { /* 缓存损坏时忽略 */ }
}
