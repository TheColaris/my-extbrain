import { useMemo } from 'react'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { cn } from '@/lib/utils'

// 全站 Markdown 渲染唯一出口：marked + DOMPurify 消毒 + github-markdown-css 皮肤
export function MdView({ content, className }: { content: string; className?: string }) {
  const html = useMemo(
    () => DOMPurify.sanitize(marked.parse(content, { async: false })),
    [content],
  )
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
