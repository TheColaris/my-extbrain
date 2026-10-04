import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { BrandMark } from '@/components/brand-mark'
import { api } from '@/lib/api'

// CLI 设备授权页（gh auth login 同款）：终端发起 → 浏览器确认 → 终端取走密钥
export function CLIAuthPage() {
  const [sp] = useSearchParams()
  const nav = useNavigate()
  const code = (sp.get('code') ?? '').toUpperCase()
  const [state, setState] = useState<'idle' | 'busy' | 'done' | 'error'>('idle')
  const [err, setErr] = useState('')

  async function approve() {
    setState('busy')
    setErr('')
    try {
      await api('/cli-auth/approve', { method: 'POST', body: JSON.stringify({ code }) })
      setState('done')
    } catch (e) {
      setState('error')
      setErr(e instanceof Error ? e.message : '授权失败')
    }
  }

  if (!code) {
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <div className="w-full max-w-md rounded-xl border-3 border-foreground bg-card p-6 text-center shadow-[4px_4px_0px_var(--shadow-color)]">
          <p className="font-extrabold">缺少授权码</p>
          <p className="mt-2 text-sm font-semibold text-muted-foreground">请在终端重新执行 extbrain auth login</p>
          <Button className="mt-4" variant="outline" onClick={() => nav('/')}>回首页</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="anim-fade-up w-full max-w-md rounded-xl border-3 border-foreground bg-card p-6 shadow-[4px_4px_0px_var(--shadow-color)]">
        <div className="flex items-center gap-2.5">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg border-3 border-foreground bg-primary shadow-[3px_3px_0px_var(--shadow-color)]">
            <BrandMark className="h-[18px] w-[18px]" />
          </span>
          <span className="font-extrabold">我的外脑 · CLI 接入</span>
        </div>

        {state === 'done' ? (
          <div className="anim-pop mt-5 rounded-lg border-3 border-foreground bg-[var(--neon-green)] p-4 text-center">
            <p className="font-extrabold">✓ 已授权</p>
            <p className="mt-1.5 text-sm font-semibold">回到终端继续（已自动完成配置），此页可关闭。</p>
          </div>
        ) : (
          <>
            <p className="mt-5 text-sm font-semibold leading-relaxed">
              你的终端正在请求接入这个账号，授权后将为它签发一把专属 API 密钥。
            </p>
            <div className="mt-3 rounded-lg border-[2.5px] border-foreground bg-background px-4 py-3 text-center">
              <div className="text-xs font-bold text-muted-foreground">授权码</div>
              <div className="mt-1 font-mono text-2xl font-extrabold tracking-[0.3em]">{code}</div>
            </div>
            {state === 'error' && (
              <p className="anim-pop mt-3 rounded-lg border-3 border-foreground bg-destructive px-3 py-2 text-sm font-bold text-destructive-foreground">{err}</p>
            )}
            <div className="mt-4 flex gap-2.5">
              <Button className="flex-1" onClick={approve} disabled={state === 'busy'}>
                {state === 'busy' ? <span className="bk-loader" /> : '授权并签发密钥'}
              </Button>
              <Button variant="outline" onClick={() => nav('/')}>取消</Button>
            </div>
            <p className="mt-3 text-center text-xs font-semibold text-muted-foreground">密钥只会在终端本地保存，服务端仅存哈希。</p>
          </>
        )}
      </div>
    </div>
  )
}
