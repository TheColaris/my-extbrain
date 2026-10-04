import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { BrandMark } from '@/components/brand-mark'
import { MdView } from '@/components/md-view'
import { ApiError, getToken, sharesApi, type ShareView } from '@/lib/api'
import { encNotePath } from '@/components/note-modal'
import { undoToast } from '@/components/undo-toast'
import { downloadMd } from '@/lib/download'

// 公开分享只读页 /share/:token（游客与登录态都可访问，独立于 Layout）：
// 快照/live 内容 + 分享元信息 + 「创建自己的外脑」导流；失效三分文案（过期/源删除/停止）。
// 分享的边界：无需登录即可查看、可随时停止、默认过期。
export function ShareViewPage() {
  const { token = '' } = useParams()
  const nav = useNavigate()
  const [phase, setPhase] = useState<'loading' | 'done' | 'invalid'>('loading')
  const [view, setView] = useState<ShareView | null>(null)
  const [reason, setReason] = useState<'expired' | 'source_deleted' | 'not_found' | 'rate_limited'>('not_found')
  const [copyState, setCopyState] = useState<'idle' | 'busy' | 'done'>('idle')
  const [copiedPath, setCopiedPath] = useState('')

  useEffect(() => {
    let alive = true
    setPhase('loading')
    sharesApi
      .publicGet(token)
      .then((v) => {
        if (!alive) return
        setView(v)
        setPhase('done')
      })
      .catch((e) => {
        if (!alive) return
        if (e instanceof ApiError) {
          if (e.status === 429) setReason('rate_limited') // 限流≠失效：别把 429 显示成「链接不存在」
          else if (e.code === 'share_expired') setReason('expired')
          else if (e.code === 'source_deleted') setReason('source_deleted')
          else setReason('not_found')
        }
        setPhase('invalid')
      })
    return () => { alive = false }
  }, [token])

  // 复制到我的知识库：游客先登录（回跳本页），登录态一键复制（同名自动后缀，不覆盖）
  const doCopy = async () => {
    if (!getToken()) {
      nav('/login?redirect=' + encodeURIComponent(`/share/${token}`))
      return
    }
    setCopyState('busy')
    try {
      const r = await sharesApi.copy(token)
      setCopiedPath(r.note.path)
      setCopyState('done')
    } catch (e) {
      setCopyState('idle')
      if (e instanceof ApiError && e.status === 401) return // api() 已跳首页
      undoToast(e instanceof ApiError ? e.message : '复制失败，请稍后重试')
    }
  }

  return (
    <div className="relative flex min-h-screen flex-col overflow-hidden">
      {/* 品牌条（左上；点击回主页） */}
      <div className="relative z-10 flex items-center gap-2.5 px-7 py-5">
        <button
          type="button" aria-label="回主页" onClick={() => nav('/')}
          className="bk-interactive flex h-[30px] w-[30px] cursor-pointer items-center justify-center rounded-[7px] border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)] hover:shadow-[5px_5px_0px_var(--shadow-color)]"
        >
          <BrandMark className="h-[17px] w-[17px]" />
        </button>
        <span className="text-sm font-extrabold">我的外脑</span>
        <span className="font-mono text-[11px] font-semibold text-muted-foreground">my-extbrain</span>
        <span className="ml-auto inline-flex h-6 items-center rounded border-2 border-foreground bg-[var(--neon-blue)] px-2 text-[11px] font-extrabold">分享的笔记</span>
      </div>

      <div className="relative z-10 flex flex-1 items-start justify-center px-6 pb-16 max-md:px-3.5">
        {phase === 'loading' && (
          <div className="mt-16 w-full max-w-[760px]">
            <div className="anim-fade-up border-3 border-foreground bg-card p-8 shadow-[6px_6px_0px_var(--shadow-color)]">
              <span className="mb-4 block h-7 w-[46%] rounded bg-foreground/10" />
              <span className="mb-3 block h-3.5 w-[30%] rounded bg-foreground/10" />
              <div className="h-4" />
              <span className="mb-3 block h-3.5 w-[92%] rounded bg-foreground/10" />
              <span className="mb-3 block h-3.5 w-[86%] rounded bg-foreground/10" />
              <span className="block h-3.5 w-[64%] rounded bg-foreground/10" />
            </div>
          </div>
        )}

        {phase === 'done' && view && (
          <div className="anim-fade-up mt-8 w-full max-w-[760px] max-md:mt-4">
            <div className="border-3 border-foreground bg-card shadow-[8px_8px_0px_var(--shadow-color)] max-md:shadow-[5px_5px_0px_var(--shadow-color)]">
              {/* 标题区 */}
              <div className="border-b-3 border-foreground px-7 pb-5 pt-6 max-md:px-5">
                <h1 className="break-words text-[26px] font-extrabold leading-snug tracking-tight max-md:text-[21px]">{view.title || '未命名笔记'}</h1>
                <div className="mt-3 flex flex-wrap items-center gap-2.5 text-[11.5px] font-semibold text-muted-foreground">
                  <span className="rounded border-2 border-foreground bg-primary px-1.5 py-0.5 font-extrabold text-foreground">
                    {view.mode === 'live' ? '跟随更新' : '快照'}
                  </span>
                  <span>更新于 {fmtFull(view.update_time)}</span>
                  {(view.tags ?? []).map((t) => (
                    <span key={t} className="inline-flex h-[19px] items-center rounded border-[1.5px] border-foreground bg-background px-1.5 text-[10.5px] font-bold text-foreground">{t}</span>
                  ))}
                </div>
              </div>
              {/* 正文（与站内同一渲染器） */}
              <div className="px-7 py-6 max-md:px-5">
                <MdView content={view.content} className="rounded-none border-0 bg-transparent p-0 shadow-none" />
              </div>
            </div>

            {/* 页脚：来源 + 复制到我的知识库 + 导流 */}
            <div className="mt-7 flex flex-wrap items-center gap-3 pb-4 text-[12.5px] font-semibold text-muted-foreground">
              <span>由「我的外脑」分享{view.expire_time ? ` · ${fmtDate(view.expire_time)} 前有效` : ' · 长期有效'}</span>
              <div className="ml-auto flex flex-wrap items-center gap-2.5">
                {copyState === 'done' ? (
                  <>
                    <span className="font-mono text-[11.5px]">已存入 {copiedPath}</span>
                    <Button size="sm" data-role="share-open-copy" onClick={() => nav('/notes/' + encNotePath(copiedPath))}>打开已复制的笔记</Button>
                  </>
                ) : (
                  <Button size="sm" data-role="share-copy-note" disabled={copyState === 'busy'} onClick={doCopy}>
                    {copyState === 'busy' ? <span className="bk-loader" /> : '复制到我的知识库'}
                  </Button>
                )}
                <Button size="sm" variant="outline" data-role="share-download" className="normal-case" onClick={() => downloadMd(view.title || '未命名笔记', view.content)}>下载 .md</Button>
                <Button size="sm" variant="outline" onClick={() => nav('/')}>创建自己的外脑</Button>
              </div>
            </div>
          </div>
        )}

        {phase === 'invalid' && (
          <div className="anim-pop mt-24 w-full max-w-[560px] text-center max-md:mt-16">
            <div className="mx-auto mb-4 flex h-[64px] w-[64px] items-center justify-center rounded-xl border-3 border-foreground bg-[var(--neon-pink)] shadow-[5px_5px_0px_var(--shadow-color)]">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-7 w-7">
                <circle cx="12" cy="12" r="9" /><path d="M9.4 9.2a2.7 2.7 0 0 1 5.2 1c0 1.8-2.6 2.2-2.6 3.8" /><circle cx="12" cy="17.6" r="0.4" fill="currentColor" />
              </svg>
            </div>
            <h1 className="text-[22px] font-extrabold tracking-tight">{INVALID_TEXT[reason].title}</h1>
            <p className="mt-2 text-sm font-semibold text-muted-foreground">{INVALID_TEXT[reason].desc}</p>
            <div className="mt-6 flex justify-center gap-3">
              {reason === 'rate_limited' && (
                <Button size="lg" data-role="share-retry" onClick={() => window.location.reload()}>重试</Button>
              )}
              <Button size="lg" variant={reason === 'rate_limited' ? 'outline' : 'default'} onClick={() => nav('/')}>回主页</Button>
            </div>
          </div>
        )}
      </div>

      {/* 几何装饰（与 403/404 同族） */}
      <div className="pointer-events-none absolute -bottom-16 -right-14 z-0 h-[210px] w-[210px] rounded-full border-3 border-foreground bg-[var(--neon-green)] shadow-[8px_8px_0px_var(--shadow-color)] max-md:h-[150px] max-md:w-[150px]" />
      <div className="pointer-events-none absolute -left-10 top-[38%] z-0 h-[88px] w-[88px] -rotate-12 border-3 border-foreground bg-[var(--neon-orange)] shadow-[6px_6px_0px_var(--shadow-color)] max-md:hidden" />
    </div>
  )
}

const INVALID_TEXT = {
  expired: { title: '分享已过期', desc: '分享者设置了有效期，链接已自动失效。' },
  source_deleted: { title: '原笔记已删除', desc: '分享者删除了这篇笔记，分享随之失效。' },
  not_found: { title: '分享不存在或已停止', desc: '链接可能被停止分享，或地址不完整。' },
  rate_limited: { title: '访问太频繁', desc: '分享链接被大量访问，请稍等片刻再重试。' },
} as const

function fmtFull(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
function fmtDate(iso: string): string {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}
