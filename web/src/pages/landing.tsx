import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { copyText } from '@/lib/clipboard'
import { BrandMark } from '@/components/brand-mark'
import { useState } from 'react'

// 官网 Landing（/ 恒为官网，不鉴权）；「进入我的外脑」=已登录跳 /dashboard，未登录跳登录（登录后回跳）
export function LandingPage({ user }: { user: { nick_name: string } | null }) {
  const nav = useNavigate()
  const [copied, setCopied] = useState(false)
  const [loginCopied, setLoginCopied] = useState(false)
  const installCmd = `curl -fsSL ${location.origin}/install.sh | sh`
  const loginCmd = `extbrain auth login --server ${location.origin}`

  return (
    <div className="min-h-screen">
      {/* 顶栏 */}
      <header className="sticky top-0 z-10 border-b-3 border-foreground bg-card">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2.5 font-extrabold">
            <span className="flex h-9 w-9 items-center justify-center rounded-lg border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)]">
              <BrandMark className="h-[19px] w-[19px]" />
            </span>
            我的外脑
            <span className="font-mono text-xs font-semibold text-muted-foreground">my-extbrain</span>
          </div>
          {user ? (
            <Button size="sm" onClick={() => nav('/dashboard')}>进入我的外脑 →</Button>
          ) : (
            <Button size="sm" variant="outline" onClick={() => nav('/login')}>登录</Button>
          )}
        </div>
      </header>

      {/* Hero */}
      <section className="relative overflow-hidden">
        <div className="pointer-events-none absolute -right-14 top-10 h-52 w-52 rounded-full border-3 border-foreground bg-[var(--neon-pink)] shadow-[8px_8px_0px_var(--shadow-color)] max-lg:hidden" />
        <div className="pointer-events-none absolute right-28 top-48 h-20 w-20 rotate-12 border-3 border-foreground bg-[var(--neon-blue)] shadow-[6px_6px_0px_var(--shadow-color)] max-lg:hidden" />
        <div className="mx-auto max-w-6xl px-6 py-16 sm:py-20">
          <h1 className="anim-fade-up text-3xl font-extrabold leading-snug tracking-tight sm:text-5xl sm:leading-[1.3]">
            记性交给 AI，
            <br />
            外脑留给自己。
          </h1>
          <p className="anim-fade-up d-1 mt-5 text-base font-semibold leading-relaxed text-muted-foreground sm:text-lg sm:leading-8">
            待办、便签、Markdown 知识库——
            <br />
            AI 通过一条命令替你记、替你查，你负责想和用。
          </p>
          <div className="anim-fade-up d-2 mt-8 flex flex-wrap gap-3.5">
            <Button size="lg" data-role="enter-app" onClick={() => nav(user ? '/dashboard' : '/login?redirect=/dashboard')}>进入我的外脑 →</Button>
            {!user && <Button size="lg" variant="outline" onClick={() => nav('/login?tab=reg')}>开始使用</Button>}
            <Button size="lg" variant="outline" onClick={() => document.getElementById('demo')?.scrollIntoView({ behavior: 'smooth' })}>
              看看怎么玩 ↓
            </Button>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-6xl px-6">
        {/* 三卡 */}
        <section className="grid gap-5 md:grid-cols-3">
          {[
            { tag: '待办', tagCls: 'bg-primary', title: '要做的事，说一声就记下', desc: '跟 AI 说「记一下周五交周报」，它写进你的列表。状态随手流转，到期提醒推到你手机。' },
            { tag: '便签', tagCls: 'bg-[var(--neon-blue)]', title: '一闪而过的想法先扔进来', desc: '一句话也值得记。时间流排布，可置顶、可搜、可随时整条复制走。' },
            { tag: '知识库', tagCls: 'bg-[var(--neon-purple)]', title: '资料整理好，存成你自己的库', desc: 'Markdown 按目录归档，全文可搜。问 AI「上次那个资料呢」，它先查你的库再回答。' },
          ].map((f, i) => (
            <div key={f.tag} className={`anim-fade-up d-${i + 1} rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[6px_6px_0px_var(--shadow-color)]`}>
              <span className={`inline-flex h-[26px] items-center rounded-md border-[2.5px] border-foreground px-2.5 text-xs font-extrabold ${f.tagCls}`}>{f.tag}</span>
              <h3 className="mt-3 text-[17px] font-extrabold">{f.title}</h3>
              <p className="mt-2 text-[13.5px] font-semibold leading-relaxed text-muted-foreground">{f.desc}</p>
            </div>
          ))}
        </section>

        {/* AI 演示 */}
        <section id="demo" className="mt-14 scroll-mt-20">
          <h2 className="flex items-center gap-2.5 text-xl font-extrabold sm:text-2xl">
            <span className="h-3 w-3 border-2 border-foreground bg-[var(--neon-purple)] max-sm:hidden" />
            AI 是这样替你记的
          </h2>
          <p className="mt-2 text-[13.5px] font-semibold text-muted-foreground">装好 CLI 后，直接对你的 AI 说「帮我记一下」——剩下的它自己会做。</p>
          <div className="mt-4 overflow-x-auto rounded-xl border-3 border-foreground bg-[#111] px-5 py-4 font-mono text-[13px] leading-loose text-[#e4e4e7] shadow-[4px_4px_0px_var(--shadow-color)]">
            <div><span className="text-primary">$</span> extbrain todo add "周五前交周报" --due 2026-10-09</div>
            <div className="text-[var(--neon-green)]">✓ #7 周五前交周报</div>
            <div className="h-2.5" />
            <div><span className="text-primary">$</span> extbrain note push 调研笔记.md --path ai/glm-使用笔记.md</div>
            <div className="text-[var(--neon-green)]">✓ ai/glm-使用笔记.md（4.2 KB）</div>
            <div className="h-2.5" />
            <div><span className="text-primary">$</span> extbrain search "Supabase"</div>
            <div className="text-gray-400">● ai/supabase-坑.md</div>
            <div className="text-gray-400">  …session pooler 下 prepared statements 要禁用…</div>
          </div>

          {/* 接入 AI 三步走（CLI + Skill 从本站直接下载，地址随当前访问域名动态生成） */}
          <div className="mt-4 rounded-xl border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[6px_6px_0px_var(--shadow-color)] sm:p-5">
            <div className="text-sm font-extrabold">接入 AI · 三步走</div>

            <div className="mt-3 space-y-3">
              <div>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-xs font-extrabold"><span className="mr-1.5 inline-flex h-5 w-5 items-center justify-center rounded border-2 border-foreground bg-primary text-[11px]">1</span>安装 CLI 与 Skill</span>
                  <Button size="sm" variant="outline" onClick={async () => { if (await copyText(installCmd)) { setCopied(true); setTimeout(() => setCopied(false), 1500) } }}>
                    {copied ? '✓ 已复制' : '复制命令'}
                  </Button>
                </div>
                <div className="mt-1.5 overflow-x-auto rounded-lg border-[2.5px] border-foreground bg-[#111] px-3.5 py-2.5 font-mono text-[13px] text-[#e4e4e7]">
                  <span className="text-primary">$</span> {installCmd}
                </div>
              </div>

              <div>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-xs font-extrabold"><span className="mr-1.5 inline-flex h-5 w-5 items-center justify-center rounded border-2 border-foreground bg-[var(--neon-purple)] text-[11px]">2</span>登录（会自动打开浏览器，点一下「授权」即可）</span>
                  <Button size="sm" variant="outline" onClick={async () => { if (await copyText(loginCmd)) { setLoginCopied(true); setTimeout(() => setLoginCopied(false), 1500) } }}>
                    {loginCopied ? '✓ 已复制' : '复制命令'}
                  </Button>
                </div>
                <div className="mt-1.5 overflow-x-auto rounded-lg border-[2.5px] border-foreground bg-[#111] px-3.5 py-2.5 font-mono text-[13px] text-[#e4e4e7]">
                  <span className="text-primary">$</span> {loginCmd}
                </div>
              </div>

              <div className="flex items-center gap-2 text-xs font-extrabold">
                <span className="inline-flex h-5 w-5 items-center justify-center rounded border-2 border-foreground bg-[var(--neon-green)] text-[11px]">3</span>
                对你的 AI 说：「帮我记一下……」「查查我收藏的……」
              </div>
            </div>

            <div className="mt-3 flex flex-wrap gap-2">
              {['extbrain_darwin_arm64.tar.gz', 'extbrain_linux_amd64.tar.gz', 'extbrain_linux_arm64.tar.gz', 'extbrain-skill.tar.gz'].map((f) => (
                <span key={f} className="rounded-md border-2 border-foreground bg-background px-2 py-0.5 font-mono text-[11px] font-bold">{f}</span>
              ))}
            </div>
            <p className="mt-2 text-xs font-semibold text-muted-foreground">脚本自动识别系统架构；地址随当前域名自动生成，换域名无需改文档。</p>
          </div>
        </section>

        {/* 三步 */}
        <section className="mt-14">
          <h2 className="flex items-center gap-2.5 text-xl font-extrabold sm:text-2xl">
            <span className="h-3 w-3 border-2 border-foreground bg-[var(--neon-green)] max-sm:hidden" />
            三分钟上手
          </h2>
          <div className="mt-4 grid gap-5 md:grid-cols-3">
            {[
              { n: '1', nCls: 'bg-card', title: '记一条东西', desc: '待办管要做的事，便签收碎片想法——跟用备忘录一样简单。' },
              { n: '2', nCls: 'bg-[var(--neon-purple)]', title: '攒成知识库', desc: '资料存成 Markdown，按目录归档，全文可搜，整篇可复制。' },
              { n: '3', nCls: 'bg-foreground text-background', hot: true, title: '让 AI 替你记', desc: '一条命令装好 CLI（见下方安装区）→ 面板签发 API 密钥 → 把登录命令发给你的 AI（Claude Code / ZCode 等），从此它替你记、替你查。' },
            ].map((s2) => (
              <div key={s2.n} className={`relative rounded-xl border-3 border-foreground p-4 shadow-[4px_4px_0px_var(--shadow-color)] transition-shadow duration-300 hover:shadow-[6px_6px_0px_var(--shadow-color)] ${s2.hot ? 'bg-primary' : 'bg-card'}`}>
                {s2.hot && <span className="absolute -top-3 right-2.5 rounded border-2 border-foreground bg-destructive px-2 py-px text-[11px] font-extrabold text-destructive-foreground">核心玩法</span>}
                <div className={`flex h-[30px] w-[30px] items-center justify-center rounded-lg border-3 border-foreground font-extrabold ${s2.nCls}`}>{s2.n}</div>
                <h3 className="mt-2.5 text-[15px] font-extrabold">{s2.title}</h3>
                <p className="mt-1.5 text-[12.5px] font-semibold leading-relaxed text-muted-foreground">{s2.desc}</p>
              </div>
            ))}
          </div>
        </section>
      </div>

      {/* 页脚 */}
      <footer className="mt-14 border-t-3 border-foreground bg-card">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-2.5 px-6 py-5 text-[12.5px] font-bold text-muted-foreground">
          <span>Go + React · Docker 自托管 · 数据在自己手里</span>
          <span>开源（MIT）· my-extbrain</span>
        </div>
      </footer>
    </div>
  )
}
