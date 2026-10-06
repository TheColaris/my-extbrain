import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useParams, useSearchParams } from 'react-router-dom'
import { Layout } from '@/components/layout'
import { BrandMark } from '@/components/brand-mark'
import { KeysPage } from '@/pages/keys'
import { LogsPage } from '@/pages/logs'
import { TodosPage } from '@/pages/todos'
import { MemosPage } from '@/pages/memos'
import { DashboardPage } from '@/pages/dashboard'
import { SettingsPage } from '@/pages/settings'
import { NotesPage } from '@/pages/notes'
import { LandingPage } from '@/pages/landing'
import { CLIAuthPage } from '@/pages/cli-auth'
import { NotLoggedInPage } from '@/pages/not-logged-in'
import { ShareViewPage } from '@/pages/share-view'
import { NotFoundPage } from '@/pages/not-found'
import { NoteSoloView } from '@/components/note-modal'
import { SearchPage } from '@/pages/search'
import { TrashPage } from '@/pages/trash'
import { NotifyLogPage } from '@/pages/notify-log'
import { GuidePage } from '@/pages/guide'
import { AdminPage } from '@/pages/admin'
import { OpsPage } from '@/pages/ops'
import { ApiError, accountApi, authApi, getToken, keysApi, setToken, type User } from '@/lib/api'
import { ErrorBoundary } from '@/components/error-boundary'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

type Mode = 'login' | 'reg'

function RedirectToLogin() {
  const target = encodeURIComponent(location.pathname + location.search)
  return <Navigate to={'/login?redirect=' + target} replace />
}

// 旧详情页路由 → 单篇直达（深链接兼容：/notes/view?path=x → /notes/x）
function NoteViewRedirect() {
  const [sp] = useSearchParams()
  const p = sp.get('path')
  return <Navigate to={p ? '/notes/' + p.split('/').map(encodeURIComponent).join('/') : '/notes'} replace />
}

// /notes/* 单篇直达路由：edit=1 → 库页并直入该篇编辑；否则单篇浮窗视图
// /notes 精确路由：?path= 旧参数深链接 → 重定向到 /notes/<路径>；无参 = 知识库
function NotesRoute() {
  const [sp] = useSearchParams()
  const p = sp.get('path')
  if (p) return <Navigate to={'/notes/' + p.split('/').map(encodeURIComponent).join('/')} replace />
  return <NotesPage />
}

function NoteSoloRoute() {
  const splat = useParams()['*'] ?? ''
  const [sp] = useSearchParams()
  if (!splat) return <Navigate to="/notes" replace />
  // ?edit=1：单篇直达并直接进编辑态（浮窗内编辑）
  return <NoteSoloView path={splat} initialMode={sp.get('edit') === '1' ? 'edit' : 'read'} />
}

// 登录态再入 /login：按 redirect 参数回原页（仅站内路径），无则回主页。
// 声明式回跳——取代 AuthPage 里的命令式 nav(redirect)：那会在 setUser 重渲染时
// 被 /login 路由自身的 Navigate 抢跑，回跳从未生效（本地 E2E 实测定位）。
function LoginDoneRedirect() {
  const [sp] = useSearchParams()
  const to = sp.get('redirect') ?? '/dashboard'
  return <Navigate to={to.startsWith('/') && !to.startsWith('//') ? to : '/dashboard'} replace />
}

export default function App() {
  const [user, setUser] = useState<User | null>(null)
  const [booting, setBooting] = useState(true)

  useEffect(() => {
    const boot = async () => {
      if (getToken()) {
        const cached = localStorage.getItem('extbrain_user')
        if (cached) {
          try { setUser(JSON.parse(cached)) } catch { /* 忽略损坏缓存 */ }
        }
        // 探活验真：服务端明确拒绝（401/403，api() 内已清凭据并跳 /）才算登录失效；
        // 网络/服务瞬断只降级为游客视图，保留本地凭据——否则一次抖动就静默掉登录
        try {
          await keysApi.list()
        } catch {
          setUser(null)
        }
        // 静默刷新用户缓存（发版新增字段如 is_admin 需要回填；失败保留旧缓存不阻断）
        accountApi.me().then((m) => {
          localStorage.setItem('extbrain_user', JSON.stringify(m))
          setUser((prev) => (prev ? m : prev))
        }).catch(() => {})
      }
      setBooting(false)
    }
    boot()
  }, [])

  // 账户资料变更（头像/昵称）→ 重读缓存刷新侧栏（设置页 syncUserCache 派发）
  useEffect(() => {
    const onUserChanged = () => {
      const cached = localStorage.getItem('extbrain_user')
      if (cached) {
        try { setUser(JSON.parse(cached)) } catch { /* 忽略损坏缓存 */ }
      }
    }
    window.addEventListener('extbrain:user-changed', onUserChanged)
    return () => window.removeEventListener('extbrain:user-changed', onUserChanged)
  }, [])

  if (booting)
    return (
      <div className="flex min-h-screen items-center justify-center">
        <span className="bk-loader" style={{ width: 22, height: 22, borderWidth: 4 }} />
      </div>
    )

  const logout = () => {
    setToken(null)
    localStorage.removeItem('extbrain_user')
    setUser(null)
  }

  // 单一 BrowserRouter：登录态切换不再整体换 Router。
  // 旧结构（游客/登录态两套 <BrowserRouter> 条件渲染）在登录瞬间卸载旧 Router，
  // AuthPage 里排程中的 nav(redirect) 随之丢失，新 Router 按旧 URL 把 /login 重定向到 /，
  // 「登录/注册后回到原页面」从未真正生效（本地 E2E 实测定位）。
  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/login"
          element={user ? <LoginDoneRedirect /> : <AuthPage onAuthed={(u) => { localStorage.setItem('extbrain_user', JSON.stringify(u)); setUser(u) }} />}
        />
        <Route path="/cli-auth" element={user ? <ErrorBoundary><CLIAuthPage /></ErrorBoundary> : <RedirectToLogin />} />
        {/* 公开分享只读页：游客与登录态都可访问（分享的边界=无需登录/可停止/默认过期） */}
        <Route path="/share/:token" element={<ErrorBoundary><ShareViewPage /></ErrorBoundary>} />
        {/* 官网 Landing：/ 恒为官网（不鉴权）；进入应用走 /dashboard */}
        <Route path="/" element={<LandingPage user={user} />} />
        {user ? (
          <>
            <Route element={<ErrorBoundary><Layout user={user} onLogout={logout} /></ErrorBoundary>}>
              <Route path="/dashboard" element={<DashboardPage nick={user.nick_name} />} />
              <Route path="/todos" element={<TodosPage />} />
              <Route path="/memos" element={<MemosPage />} />
              <Route path="/notes" element={<NotesRoute />} />
              <Route path="/notes/view" element={<NoteViewRedirect />} />
              <Route path="/notes/*" element={<NoteSoloRoute />} />
              <Route path="/keys" element={<KeysPage />} />
              <Route path="/logs" element={<LogsPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/search" element={<SearchPage />} />
              <Route path="/trash" element={<TrashPage />} />
              <Route path="/notify-log" element={<NotifyLogPage />} />
              <Route path="/guide" element={<GuidePage />} />
              {/* 平台管理（仅管理员；后端 AdminOnly 二次拦截） */}
              {user.is_admin && <Route path="/admin" element={<AdminPage />} />}
              {user.is_admin && <Route path="/admin/ops" element={<OpsPage />} />}
            </Route>
            {/* 登录态未知路径 → 404 页（独立于 Layout，与 403 同族；游客态在下方走 403） */}
            <Route path="*" element={<NotFoundPage />} />
          </>
        ) : (
          <>
            {/* 游客访问 /dashboard → 登录（登录后回跳 /dashboard） */}
            <Route path="/dashboard" element={<RedirectToLogin />} />
            {/* 操作手册对游客公开（手册=产品说明书；游客态自带顶栏，无侧栏） */}
            <Route path="/guide" element={<ErrorBoundary><GuidePage guest /></ErrorBoundary>} />
            {/* 游客访问受保护路由（含未知路径）→ 403 未登录拦截页（公开路由仅 / /login /cli-auth /guide） */}
            <Route path="*" element={<NotLoggedInPage />} />
          </>
        )}
      </Routes>
    </BrowserRouter>
  )
}

/* ============ 登录 / 注册（左品牌区 + 右表单区，带动效） ============ */
function AuthPage({ onAuthed }: { onAuthed: (u: User) => void }) {
  const [sp] = useSearchParams()
  const [mode, setMode] = useState<Mode>(sp.get('tab') === 'reg' ? 'reg' : 'login')
  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [emailCode, setEmailCode] = useState('')
  const [cool, setCool] = useState(0)
  const [sendBusy, setSendBusy] = useState(false)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  // 验证码重发倒计时（60s）
  useEffect(() => {
    if (cool <= 0) return
    const t = setTimeout(() => setCool((c) => c - 1), 1000)
    return () => clearTimeout(t)
  }, [cool])

  async function sendCode() {
    setErr('')
    if (!account.trim()) {
      setErr('请先输入邮箱')
      return
    }
    setSendBusy(true)
    try {
      await authApi.sendEmailCode(account.trim())
      setCool(60)
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '网络异常，请稍后重试')
    } finally {
      setSendBusy(false)
    }
  }

  async function submit() {
    setErr('')
    if (mode === 'reg') {
      if (!account.trim()) {
        setErr('请输入邮箱')
        return
      }
      if (!emailCode.trim()) {
        setErr('请输入邮箱验证码')
        return
      }
      if (password !== confirm) {
        setErr('两次密码不一致')
        return
      }
    }
    setBusy(true)
    try {
      const resp =
        mode === 'login'
          ? await authApi.login(account.trim(), password)
          : await authApi.register(account.trim(), password, emailCode.trim())
      setToken(resp.token)
      onAuthed(resp.user)
      // 回跳交给路由层：登录态的 /login → LoginDoneRedirect 读 redirect 参数跳原页
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '网络异常，请稍后重试')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen">
      {/* 左侧品牌区（黄底 + 脑图 + 标语 + badges + 几何装饰） */}
      <div className="relative hidden flex-1 flex-col justify-center overflow-hidden bg-primary p-16 lg:flex">
        <BrandMark className="anim-fade-up mb-7 h-14 w-14" />
        <h1 className="anim-fade-up d-1 text-4xl font-extrabold leading-snug tracking-tight">
          记性交给 AI，
          <br />
          外脑留给自己。
        </h1>
        <p className="anim-fade-up d-2 mt-4 text-lg font-semibold text-foreground/80">
          待办 · 便签 · 知识库，一处存放，处处可复制。
        </p>
        <div className="anim-fade-up d-3 mt-9 flex flex-wrap gap-2">
          {['CLI', 'Skill', 'API Key', 'Open API'].map((t) => (
            <span key={t} className="border-3 border-foreground bg-background px-3 py-1 text-xs font-bold uppercase tracking-wide">
              {t}
            </span>
          ))}
        </div>
        {/* 几何装饰（右下） */}
        <div className="anim-pop d-4 pointer-events-none absolute -bottom-10 -right-10 h-56 w-56 rounded-full border-3 border-foreground bg-[var(--neon-pink)] shadow-[8px_8px_0px_var(--shadow-color)]" />
        <div className="anim-pop d-5 pointer-events-none absolute bottom-24 right-18 h-22 w-22 rotate-12 border-3 border-foreground bg-[var(--neon-blue)] shadow-[6px_6px_0px_var(--shadow-color)]" style={{ width: 88, height: 88 }} />
      </div>

      {/* 右侧表单区 */}
      <div className="flex flex-1 items-center justify-center p-10">
        <div className="w-full max-w-sm space-y-6">
          <div className="anim-slide-in">
            <Tabs value={mode} onValueChange={(v) => { setMode(v as Mode); setErr('') }}>
              <TabsList className="w-full">
                <TabsTrigger value="login" className="flex-1">登录</TabsTrigger>
                <TabsTrigger value="reg" className="flex-1">注册</TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
          {/* key=mode：切换时重挂载触发入场动画 */}
          <div key={mode} className="anim-fade-up space-y-4">
            <div className="space-y-2">
              <Label htmlFor="account">{mode === 'login' ? '手机号 / 邮箱' : '邮箱'}</Label>
              <Input
                id="account"
                value={account}
                onChange={(e) => setAccount(e.target.value)}
                placeholder={mode === 'login' ? '139… or you@example.com' : 'you@example.com'}
                onKeyDown={(e) => e.key === 'Enter' && submit()}
              />
            </div>
            {mode === 'reg' && (
              <div className="space-y-2">
                <Label htmlFor="code">验证码</Label>
                <div className="flex gap-2">
                  <Input
                    id="code"
                    value={emailCode}
                    onChange={(e) => setEmailCode(e.target.value)}
                    placeholder="6 位数字"
                    maxLength={6}
                    className="font-mono"
                    onKeyDown={(e) => e.key === 'Enter' && submit()}
                  />
                  <Button type="button" variant="outline" disabled={cool > 0 || sendBusy} onClick={sendCode} className="min-w-[112px] shrink-0">
                    {sendBusy ? <span className="bk-loader" /> : cool > 0 ? `${cool}s 后重发` : '发送验证码'}
                  </Button>
                </div>
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input id="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={mode === 'reg' ? '至少 8 位' : '••••••••'} onKeyDown={(e) => e.key === 'Enter' && submit()} />
            </div>
            {mode === 'reg' && (
              <div className="space-y-2">
                <Label htmlFor="confirm">确认密码</Label>
                <Input id="confirm" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder="再输入一次" onKeyDown={(e) => e.key === 'Enter' && submit()} />
              </div>
            )}
          </div>
          {err && (
            <p className="anim-pop border-3 border-foreground bg-destructive px-3 py-2 text-sm font-bold text-destructive-foreground">
              {err}
            </p>
          )}
          <Button className="w-full" size="lg" disabled={busy} onClick={submit}>
            {busy ? <span className="bk-loader" /> : mode === 'login' ? '登录' : '注册'}
          </Button>
        </div>
      </div>
    </div>
  )
}
