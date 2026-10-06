import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MdView } from '@/components/md-view'
import { BrandMark } from '@/components/brand-mark'
import { copyText } from '@/lib/clipboard'
import { downloadMd } from '@/lib/download'

// 操作手册页（Skill 版 / MCP 版双 Tab）：
// 两个接入方式二选一——Skill 版（CLI+Skill，任何 AI 宿主）/ MCP 版（支持 MCP 的客户端直连）；
// 内容 = GET /guide.md、/guide-mcp.md（服务端已把地址注入为当前实例），可整份复制/下载扔给 AI；
// 游客与登录态都可看（手册=产品说明书，公开；登录态由 Layout 提供壳，游客态自带顶栏）；?tab=mcp 深链。
type DocTab = 'skill' | 'mcp'

const DOCS: Record<DocTab, { file: string; download: string; urlPath: string }> = {
  skill: { file: '/guide.md', download: '我的外脑-操作手册.md', urlPath: '/guide.md' },
  mcp: { file: '/guide-mcp.md', download: '我的外脑-MCP接入手册.md', urlPath: '/guide-mcp.md' },
}

export function GuidePage({ guest }: { guest?: boolean }) {
  const nav = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [tab, setTab] = useState<DocTab>(() => (searchParams.get('tab') === 'mcp' ? 'mcp' : 'skill'))
  const [docs, setDocs] = useState<Partial<Record<DocTab, string>>>({})
  const [failed, setFailed] = useState(false)
  const [copied, setCopied] = useState(false)
  const fetched = useRef<Set<DocTab>>(new Set())

  const md = docs[tab] ?? ''

  useEffect(() => {
    if (fetched.current.has(tab)) return
    fetched.current.add(tab)
    fetch(DOCS[tab].file)
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status))
        return r.text()
      })
      .then((text) => setDocs((d) => ({ ...d, [tab]: text })))
      .catch(() => {
        fetched.current.delete(tab)
        setFailed(true)
      })
  }, [tab])

  const switchTab = (t: string) => {
    setFailed(false)
    setCopied(false)
    setTab(t as DocTab)
    setSearchParams(t === 'mcp' ? { tab: 'mcp' } : {}, { replace: true })
  }

  const copy = async () => {
    if (md && (await copyText(md))) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    }
  }

  const body = (
    <>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-extrabold tracking-tight sm:text-2xl">操作手册</h1>
          <p className="mt-1 text-xs font-semibold text-muted-foreground">整份交给你的 AI，它就能替你记、替你查</p>
        </div>
        <div className="flex items-center gap-2">
          {!guest && (
            <Button size="sm" variant="outline" onClick={() => nav('/dashboard?tour=1')}>▶ 重看上手引导</Button>
          )}
          <Tabs value={tab} onValueChange={switchTab}>
            <TabsList>
              {/* normal-case：BoldKit TabsTrigger 基类 uppercase 会把「Skill」转「SKILL」 */}
              <TabsTrigger value="skill" className="normal-case">
                Skill 版
              </TabsTrigger>
              <TabsTrigger value="mcp" className="normal-case">
                MCP 版
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </div>

      {/* 二选一说明 */}
      <p className="mt-3 rounded-lg border-2 border-dashed border-foreground/40 bg-[var(--neon-yellow)]/30 px-3 py-2 text-xs font-semibold leading-relaxed">
        两种方式<strong>二选一即可</strong>，能力完全相同：<b>Skill 版</b>＝装 CLI，任何 AI 宿主都能用；<b>MCP 版</b>＝支持 MCP
        的客户端（Claude Code / ZCode / Cursor / Claude Desktop）直接连接，零 CLI 也可用。
      </p>

      {/* 操作条：复制全文 / 下载 .md —— 核心动作=把手册扔给 AI */}
      <div className="mt-4 flex flex-wrap items-center gap-3 rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
        <Button onClick={copy} disabled={!md}>
          {copied ? '✓ 已复制' : '复制全文给 AI'}
        </Button>
        <Button variant="outline" className="normal-case" disabled={!md} onClick={() => downloadMd(DOCS[tab].download, md)}>
          下载 .md
        </Button>
        <p className="min-w-[240px] flex-1 text-xs font-semibold leading-relaxed text-muted-foreground">
          地址已自动替换为你的实例地址。复制后粘贴给任何 AI；有联网能力的 AI 也可直接读取{' '}
          <code className="rounded border-2 border-foreground bg-[var(--neon-yellow)] px-1 font-mono text-[11px] font-bold">
            {location.origin}
            {DOCS[tab].urlPath}
          </code>
        </p>
      </div>

      <div className="mt-5">
        {failed ? (
          <div className="rounded-xl border-3 border-foreground bg-card p-8 text-center text-sm font-bold shadow-[4px_4px_0px_var(--shadow-color)]">
            手册加载失败，请刷新重试
          </div>
        ) : md ? (
          <MdView content={md} />
        ) : (
          <div className="flex justify-center py-16">
            <span className="bk-loader" style={{ width: 22, height: 22, borderWidth: 4 }} />
          </div>
        )}
      </div>
    </>
  )

  if (!guest) return body

  // 游客态：品牌顶栏 + 登录（手册对游客公开）
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-10 border-b-3 border-foreground bg-card">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2.5 font-extrabold">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)]">
              <BrandMark className="h-[19px] w-[19px]" />
            </span>
            我的外脑
            <span className="font-mono text-xs font-semibold text-muted-foreground">my-extbrain</span>
          </div>
          <Button size="sm" variant="outline" onClick={() => nav('/login?redirect=/guide')}>
            登录
          </Button>
        </div>
      </header>
      <div className="mx-auto max-w-6xl px-6 py-8">{body}</div>
    </div>
  )
}
