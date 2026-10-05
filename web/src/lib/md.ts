import DOMPurify from 'dompurify'
import { marked } from 'marked'

// GFM 自动链接会把 URL 之后的非空格字符一并吞进 href——中文便签里
// `https://x.com/a，后面接说明` 会整段变成链接（href 尾部带 %EF%BC%8C...），点击跳错地址。
// 覆写 url tokenizer：命中全角标点或 CJK 字符立即截断；其余分支保持 marked 内置行为
// （www. 补协议、邮箱 mailto、_backpedal 回退尾部 ASCII 标点）。
const CUT_AT = /[\u4e00-\u9fff\u3000-\u303f\uff01-\uff5e\u2026\u201c\u201d\u2018\u2019]/

marked.use({
  tokenizer: {
    url(src) {
      const cap = this.rules.inline.url.exec(src)
      if (!cap) return
      if (cap[2] === '@') {
        const text = cap[0]
        return { type: 'link', raw: text, text, href: 'mailto:' + text, autolink: true, tokens: [{ type: 'text', raw: text, text }] }
      }
      let raw = cap[0]
      const cut = raw.search(CUT_AT)
      if (cut > 0) raw = raw.slice(0, cut)
      if (!raw) return
      let prev: string
      do {
        prev = raw
        raw = this.rules.inline._backpedal.exec(raw)?.[0] ?? ''
      } while (prev !== raw)
      if (!raw) return
      const text = raw
      const href = cap[1] === 'www.' ? 'http://' + raw : raw
      return { type: 'link', raw, text, href, autolink: true, tokens: [{ type: 'text', raw: text, text }] }
    },
  },
})

/** 全站 Markdown 渲染唯一出口：marked + DOMPurify 消毒（.markdown-body 皮肤在 index.css） */
export function renderMD(content: string, opts?: { breaks?: boolean }): string {
  return DOMPurify.sanitize(marked.parse(content, { async: false, breaks: opts?.breaks ?? false }))
}
