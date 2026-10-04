import { useId } from 'react'

// 品牌符号：对称脑（圆弧构造；中缝与沟回为真实镂空——任意底色/主题自适应）
// 品牌图形：脑 + 闪电
export function BrandMark({ className = 'h-5 w-5' }: { className?: string }) {
  const maskId = useId()
  return (
    <svg viewBox="0 0 24 24" className={className} aria-hidden="true">
      <mask id={maskId}>
        <rect x="0" y="0" width="24" height="24" fill="#fff" />
        <g stroke="#000" strokeWidth="1.2" fill="none" strokeLinecap="round" transform="translate(-0.275,0)">
          <path d="M8.2 6.9C6.6 7.4 5.4 8.7 5.1 10.3" />
        </g>
        <g stroke="#000" strokeWidth="1.2" fill="none" strokeLinecap="round" transform="translate(24.275,0) scale(-1,1)">
          <path d="M8.2 6.9C6.6 7.4 5.4 8.7 5.1 10.3" />
        </g>
      </mask>
      <g mask={`url(#${maskId})`} fill="currentColor" transform="translate(1.23 0.86) scale(0.897)">
        <g transform="translate(-0.275,0)">
          <path d="M12 5.0a3.4 3.4 0 1 0-6.1.6 4.6 4.6 0 0 0-2.3 6.3 4.3 4.3 0 0 0 .9 5.9A4.4 4.4 0 1 0 12 17.6Z" />
        </g>
        <g transform="translate(24.275,0) scale(-1,1)">
          <path d="M12 5.0a3.4 3.4 0 1 0-6.1.6 4.6 4.6 0 0 0-2.3 6.3 4.3 4.3 0 0 0 .9 5.9A4.4 4.4 0 1 0 12 17.6Z" />
        </g>
      </g>
    </svg>
  )
}
