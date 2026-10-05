import { useMemo } from 'react'
import { cn } from '@/lib/utils'
import { renderMD } from '@/lib/md'

// 全站 Markdown 渲染组件出口：渲染逻辑（marked + DOMPurify）在 lib/md.ts，
// 皮肤=github-markdown-css 作用域化（.markdown-body，见 index.css）。
export function MdView({ content, className }: { content: string; className?: string }) {
  const html = useMemo(() => renderMD(content), [content])
  return (
    <div
      className={cn(
        'markdown-body',
        // neubrutalism 化：粗边卡片 + 硬阴影 + 主题色适配
        'rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]',
        '[&_.octicon]:hidden',
        className,
      )}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
