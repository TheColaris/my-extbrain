import { useCallback, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ApiError, adminApi, type EmailConfigOut, type EmailTestResult, type EmbeddingConfigOut, type EmbeddingTestResult, type IndexStatus } from '@/lib/api'
import { cn } from '@/lib/utils'

// 平台管理（仅管理员）：三 Tab——语义检索 / 索引 / 邮件服务（2026-10-06）。
// 语义检索卡（provider 默认硅基流动 + 测试连接 + 保存制）；索引卡（数字带 + 重建进度 + 维度不符告警）；
// 邮件服务卡（Resend，注册验证码发信 + 测试发信）。
// 密钥只回打码：未点「更换」时保存不带 api_key（后端语义=保持不变）。

const MODEL_PRESETS = [
  { model: 'BAAI/bge-m3', dim: 1024 },
  { model: 'Qwen/Qwen3-Embedding-0.6B', dim: 1024 },
  { model: 'BAAI/bge-large-zh-v1.5', dim: 1024 },
  { model: 'Qwen/Qwen3-Embedding-4B', dim: 2560 },
]

type AdminTab = 'retrieval' | 'index' | 'mail'

export function AdminPage() {
  const [tab, setTab] = useState<AdminTab>('retrieval')
  const [loading, setLoading] = useState(true)
  const [loadErr, setLoadErr] = useState('')
  const [cfg, setCfg] = useState<EmbeddingConfigOut | null>(null)
  // 表单草稿（保存制：改动只在点「保存」后提交）
  const [enabled, setEnabled] = useState(false)
  const [baseUrl, setBaseUrl] = useState('')
  const [model, setModel] = useState('')
  const [dim, setDim] = useState(1024)
  const [keyDraft, setKeyDraft] = useState<string | null>(null) // null=未更换
  const [test, setTest] = useState<EmbeddingTestResult | null>(null)
  const [testing, setTesting] = useState(false)
  const [saving, setSaving] = useState(false)
  const [idx, setIdx] = useState<IndexStatus | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // 邮件服务（Resend）
  const [mailCfg, setMailCfg] = useState<EmailConfigOut | null>(null)
  const [mailEnabled, setMailEnabled] = useState(false)
  const [mailFromName, setMailFromName] = useState('')
  const [mailFromAddr, setMailFromAddr] = useState('')
  const [mailKeyDraft, setMailKeyDraft] = useState<string | null>(null) // null=未更换
  const [mailTest, setMailTest] = useState<EmailTestResult | null>(null)
  const [mailTesting, setMailTesting] = useState(false)
  const [mailSaving, setMailSaving] = useState(false)

  const stopPoll = useCallback(() => {
    if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null }
  }, [])

  const startPoll = useCallback(() => {
    if (pollRef.current) return
    pollRef.current = setInterval(async () => {
      try {
        const st = await adminApi.indexStatus()
        setIdx(st)
        if (!st.progress.running) stopPoll()
      } catch { /* 轮询失败静默，下一轮再试 */ }
    }, 2000)
  }, [stopPoll])

  useEffect(() => () => stopPoll(), [stopPoll])

  const loadMail = useCallback(async () => {
    try {
      const m = await adminApi.getEmail()
      setMailCfg(m)
      setMailEnabled(m.enabled)
      setMailFromName(m.from_name)
      setMailFromAddr(m.from_address)
      setMailKeyDraft(null)
      setMailTest(m.last_test ?? null)
    } catch {
      setMailCfg(null)
    }
  }, [])

  const load = useCallback(async () => {
    try {
      const c = await adminApi.getEmbedding()
      setCfg(c)
      setEnabled(c.enabled)
      setBaseUrl(c.base_url)
      setModel(c.model)
      setDim(c.dim)
      setKeyDraft(null)
      setTest(c.last_test ?? null)
      setIdx(c.index)
      if (c.index.progress.running) startPoll()
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof ApiError ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
    await loadMail()
  }, [startPoll, loadMail])

  useEffect(() => { load() }, [load])

  const mismatch = !!idx && idx.enabled && idx.index_dim > 0 && idx.index_dim !== idx.config_dim
  const modelOptions = [...MODEL_PRESETS]
  if (model && !MODEL_PRESETS.some((p) => p.model === model)) {
    modelOptions.unshift({ model, dim })
  }

  const pickProvider = (v: string) => {
    if (v === 'sf' && cfg) {
      setBaseUrl(cfg.defaults.base_url)
      if (!MODEL_PRESETS.some((p) => p.model === model)) {
        setModel(cfg.defaults.model)
        setDim(cfg.defaults.dim)
      }
    }
  }

  const pickModel = (m: string) => {
    setModel(m)
    const p = MODEL_PRESETS.find((x) => x.model === m)
    if (p) setDim(p.dim)
  }

  const doTest = async () => {
    setTesting(true)
    try {
      const r = await adminApi.testEmbedding({
        base_url: baseUrl, model, dim,
        api_key: keyDraft?.trim() ? keyDraft.trim() : undefined,
      })
      setTest(r)
    } catch (e) {
      setTest({ ok: false, ms: 0, dim: 0, error: e instanceof ApiError ? e.message : '测试失败' })
    } finally { setTesting(false) }
  }

  const doSave = async () => {
    if (enabled && !cfg?.api_key_set && !keyDraft?.trim()) {
      toast.error('启用语义检索前需填写 API Key')
      return
    }
    setSaving(true)
    try {
      await adminApi.saveEmbedding({
        enabled, base_url: baseUrl.trim(), model: model.trim(), dim,
        api_key: keyDraft?.trim() ? keyDraft.trim() : undefined,
      })
      toast.success('已保存')
      stopPoll()
      await load()
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '保存失败')
    } finally { setSaving(false) }
  }

  const doRebuild = async () => {
    try {
      await adminApi.rebuild()
      toast.success('重建已开始')
      const st = await adminApi.indexStatus()
      setIdx(st)
      if (st.progress.running) startPoll()
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '重建失败')
    }
  }

  const doMailTest = async () => {
    setMailTesting(true)
    try {
      const r = await adminApi.testEmail()
      setMailTest(r)
      if (r.ok) toast.success(`测试邮件已发送至 ${r.to}`)
    } catch (e) {
      setMailTest({ ok: false, ms: 0, to: '', error: e instanceof ApiError ? e.message : '测试失败' })
    } finally { setMailTesting(false) }
  }

  const doMailSave = async () => {
    if (mailEnabled && !mailFromAddr.trim()) {
      toast.error('启用邮件服务前需填写发件地址')
      return
    }
    if (mailEnabled && !mailCfg?.api_key_set && !mailKeyDraft?.trim()) {
      toast.error('启用邮件服务前需填写 API Key')
      return
    }
    setMailSaving(true)
    try {
      await adminApi.saveEmail({
        enabled: mailEnabled,
        from_address: mailFromAddr.trim(),
        from_name: mailFromName.trim(),
        api_key: mailKeyDraft?.trim() ? mailKeyDraft.trim() : undefined,
      })
      toast.success('已保存')
      await loadMail()
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : '保存失败')
    } finally { setMailSaving(false) }
  }

  if (loading) {
    return (
      <div className="mx-auto max-w-3xl space-y-4">
        <h1 className="text-xl font-extrabold tracking-tight">平台管理</h1>
        {[0, 1].map((i) => (
          <div key={i} className="h-40 animate-pulse rounded-xl border-3 border-foreground/10 bg-card" />
        ))}
      </div>
    )
  }
  if (loadErr || !cfg) {
    return (
      <div className="mx-auto max-w-3xl">
        <h1 className="text-xl font-extrabold tracking-tight">平台管理</h1>
        <div className="mt-6 rounded-xl border-3 border-foreground border-l-[7px] border-l-[var(--neon-red)] bg-card p-6 text-sm font-bold shadow-[4px_4px_0px_var(--shadow-color)]">
          {loadErr || '加载失败'}
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="anim-fade-up">
        <h1 className="text-xl font-extrabold tracking-tight">平台管理</h1>
      </div>

      <div className="anim-fade-up">
        <Tabs value={tab} onValueChange={(v) => setTab(v as AdminTab)}>
          <TabsList className="w-full">
            <TabsTrigger value="retrieval" className="flex-1" data-role="tab-retrieval">语义检索</TabsTrigger>
            <TabsTrigger value="index" className="flex-1" data-role="tab-index">索引</TabsTrigger>
            <TabsTrigger value="mail" className="flex-1" data-role="tab-mail">邮件服务</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* ===== Tab 1: 语义检索配置 ===== */}
      {tab === 'retrieval' && (
        <div className="anim-fade-up rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]" data-role="emb-card">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Switch checked={enabled} onCheckedChange={setEnabled} data-role="emb-switch" />
              <span className="text-sm font-extrabold">语义检索</span>
            </div>
            {!enabled ? (
              <span className="inline-flex h-[22px] items-center rounded-md border-2 border-foreground bg-card px-2 text-[11px] font-bold" data-role="mode-badge">未启用</span>
            ) : mismatch ? (
              <span className="inline-flex h-[22px] items-center rounded-md border-2 border-foreground bg-[var(--neon-red)] px-2 text-[11px] font-bold" data-role="mode-badge">维度不符</span>
            ) : (
              <span className="inline-flex h-[22px] items-center rounded-md border-2 border-foreground bg-[var(--neon-purple)] px-2 text-[11px] font-bold" data-role="mode-badge">混合模式</span>
            )}
          </div>
          <div className="my-3 border-t-2 border-foreground" />
          <FieldRow label="服务商">
            <Select value={baseUrl === cfg.defaults.base_url ? 'sf' : 'custom'} onValueChange={pickProvider}>
              <SelectTrigger data-role="emb-provider"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="sf">硅基流动</SelectItem>
                <SelectItem value="custom">自定义（OpenAI 兼容）</SelectItem>
              </SelectContent>
            </Select>
          </FieldRow>
          <FieldRow label="接口地址">
            <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} data-role="emb-base-url" className="font-mono" />
          </FieldRow>
          <FieldRow label="模型">
            <Select value={model} onValueChange={pickModel}>
              <SelectTrigger data-role="emb-model"><SelectValue /></SelectTrigger>
              <SelectContent>
                {modelOptions.map((p) => (
                  <SelectItem key={p.model} value={p.model}>{p.model} · {p.dim} 维</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FieldRow>
          <FieldRow label="向量维度">
            <Input
              type="number" value={dim} onChange={(e) => setDim(Number(e.target.value) || 0)}
              data-role="emb-dim" className="w-28 font-mono"
            />
          </FieldRow>
          <FieldRow label="API Key">
            <div className="flex gap-2">
              {keyDraft === null ? (
                <>
                  <Input value={cfg.api_key_hint} readOnly data-role="emb-key-hint" className="font-mono" />
                  <Button size="sm" variant="outline" data-role="emb-key-edit" onClick={() => setKeyDraft('')}>更换</Button>
                </>
              ) : (
                <>
                  <Input
                    value={keyDraft} onChange={(e) => setKeyDraft(e.target.value)} placeholder="sk-..."
                    data-role="emb-key-input" autoFocus className="font-mono"
                  />
                  <Button size="sm" variant="outline" data-role="emb-key-cancel" onClick={() => setKeyDraft(null)}>取消</Button>
                </>
              )}
            </div>
          </FieldRow>
          <div className="my-3 border-t-2 border-foreground" />
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Button size="sm" variant="outline" data-role="emb-test" disabled={testing} onClick={doTest}>
                {testing ? '测试中…' : '测试连接'}
              </Button>
              {testing ? (
                <span className="inline-flex h-[26px] items-center rounded-md border-2 border-foreground bg-background px-2.5 text-xs font-extrabold">⋯ 请求中</span>
              ) : test ? (
                test.ok ? (
                  <span className="inline-flex h-[26px] items-center rounded-md border-2 border-foreground bg-[var(--neon-green)] px-2.5 text-xs font-extrabold" data-role="test-ok">
                    ✓ {test.ms}ms · {test.dim} 维
                  </span>
                ) : (
                  <span className="inline-flex h-[26px] max-w-[420px] items-center truncate rounded-md border-2 border-foreground bg-[var(--neon-red)] px-2.5 text-xs font-extrabold" data-role="test-fail" title={test.error}>
                    ✗ {test.error}
                  </span>
                )
              ) : null}
            </div>
            <Button size="sm" data-role="emb-save" disabled={saving} onClick={doSave}>保存</Button>
          </div>
        </div>
      )}

      {/* ===== Tab 2: 索引 ===== */}
      {tab === 'index' && (
        <div className="anim-fade-up" data-role="index-sec">
          {!enabled || !idx ? (
            <div className="rounded-xl border-3 border-foreground bg-card p-5 text-sm font-bold text-muted-foreground shadow-[4px_4px_0px_var(--shadow-color)]" data-role="index-empty">
              语义检索未启用
            </div>
          ) : (
            <>
              <div className="mb-3 flex items-center justify-between">
                {idx.last_index_at ? (
                  <span className="text-xs font-semibold text-muted-foreground">最近索引 {relTime(idx.last_index_at)}</span>
                ) : <span />}
                <Button size="sm" variant="outline" data-role="rebuild" disabled={idx.progress.running} onClick={doRebuild}>
                  {idx.progress.running ? '重建中…' : '重建全部索引'}
                </Button>
              </div>

              {mismatch && (
                <div className="mb-3.5 flex items-center gap-3 rounded-lg border-3 border-foreground border-l-[7px] border-l-[var(--neon-red)] bg-card px-3.5 py-2.5 shadow-[3px_3px_0px_var(--shadow-color)]" data-role="dim-warn">
                  <span className="inline-flex h-[22px] shrink-0 items-center rounded-md border-2 border-foreground bg-[var(--neon-red)] px-2 text-[11px] font-bold">维度不符</span>
                  <span className="text-[13px] font-bold">
                    索引 <span className="font-mono">{idx.index_dim}</span> 维 · 当前模型输出 <span className="font-mono">{idx.config_dim}</span> 维
                  </span>
                  <Button size="sm" variant="outline" className="ml-auto" onClick={doRebuild}>重建索引</Button>
                </div>
              )}

              <div className="grid grid-cols-4 overflow-hidden rounded-xl border-3 border-foreground bg-card shadow-[4px_4px_0px_var(--shadow-color)] max-md:grid-cols-2">
                <StatCell n={idx.notes} label="已索引笔记" />
                <StatCell n={idx.chunks} label="索引片段" />
                <StatCell n={idx.pending} label="待索引" />
                <StatCell n={idx.failed} label="失败" tone={idx.failed > 0 ? 'text-[var(--neon-red)]' : ''} />
              </div>

              {idx.progress.running && (
                <div className="mt-3.5" data-role="prog">
                  <div className="mb-1.5 flex items-center justify-between text-xs font-bold">
                    <span>重算中</span>
                    <span className="font-mono">{idx.progress.done} / {idx.progress.total}</span>
                  </div>
                  <div className="h-4 overflow-hidden rounded-lg border-2 border-foreground bg-background">
                    <div
                      className="h-full border-r-2 border-foreground bg-primary transition-all duration-300"
                      style={{ width: `${idx.progress.total > 0 ? Math.round((idx.progress.done / idx.progress.total) * 100) : 0}%` }}
                    />
                  </div>
                </div>
              )}

              {idx.failed > 0 && idx.failed_items.length > 0 && (
                <div className="mt-3.5 space-y-1" data-role="failed-list">
                  {idx.failed_items.map((f) => (
                    <div key={f.note_id} className="flex items-center gap-2 text-xs font-semibold text-muted-foreground">
                      <span className="font-mono">{f.path || `#${f.note_id}`}</span>
                      <span className="truncate text-[var(--neon-red)]">{f.error}</span>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* ===== Tab 3: 邮件服务（Resend，注册验证码发信） ===== */}
      {tab === 'mail' && (
        <div className="anim-fade-up rounded-xl border-3 border-foreground bg-card p-5 shadow-[4px_4px_0px_var(--shadow-color)]" data-role="mail-card">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Switch checked={mailEnabled} onCheckedChange={setMailEnabled} data-role="mail-switch" />
              <span className="text-sm font-extrabold">邮件服务</span>
            </div>
            <span className="inline-flex h-[22px] items-center rounded-md border-2 border-foreground bg-[var(--neon-blue)] px-2 text-[11px] font-bold">Resend</span>
          </div>
          {mailCfg === null ? (
            <div className="mt-4 text-sm font-bold text-muted-foreground">邮件配置加载失败，刷新重试</div>
          ) : (
            <>
              <div className="my-3 border-t-2 border-foreground" />
              <FieldRow label="发件人名称">
                <Input value={mailFromName} onChange={(e) => setMailFromName(e.target.value)} data-role="mail-from-name" />
              </FieldRow>
              <FieldRow label="发件地址">
                <Input value={mailFromAddr} onChange={(e) => setMailFromAddr(e.target.value)} data-role="mail-from-addr" className="font-mono" />
              </FieldRow>
              <FieldRow label="API Key">
                <div className="flex gap-2">
                  {mailKeyDraft === null ? (
                    <>
                      <Input value={mailCfg.api_key_hint || '未设置'} readOnly data-role="mail-key-hint" className="font-mono" />
                      <Button size="sm" variant="outline" data-role="mail-key-edit" onClick={() => setMailKeyDraft('')}>更换</Button>
                    </>
                  ) : (
                    <>
                      <Input
                        value={mailKeyDraft} onChange={(e) => setMailKeyDraft(e.target.value)} placeholder="re_..."
                        data-role="mail-key-input" autoFocus className="font-mono"
                      />
                      <Button size="sm" variant="outline" data-role="mail-key-cancel" onClick={() => setMailKeyDraft(null)}>取消</Button>
                    </>
                  )}
                </div>
              </FieldRow>
              <div className="my-3 border-t-2 border-foreground" />
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Button size="sm" variant="outline" data-role="mail-test" disabled={mailTesting} onClick={doMailTest}>
                    {mailTesting ? '发送中…' : '测试发信'}
                  </Button>
                  {mailTesting ? (
                    <span className="inline-flex h-[26px] items-center rounded-md border-2 border-foreground bg-background px-2.5 text-xs font-extrabold">⋯ 请求中</span>
                  ) : mailTest ? (
                    mailTest.ok ? (
                      <span className="inline-flex h-[26px] items-center rounded-md border-2 border-foreground bg-[var(--neon-green)] px-2.5 text-xs font-extrabold" data-role="mail-test-ok">
                        ✓ 已送达 {mailTest.to}
                      </span>
                    ) : (
                      <span className="inline-flex h-[26px] max-w-[420px] items-center truncate rounded-md border-2 border-foreground bg-[var(--neon-red)] px-2.5 text-xs font-extrabold" data-role="mail-test-fail" title={mailTest.error}>
                        ✗ {mailTest.error}
                      </span>
                    )
                  ) : null}
                </div>
                <Button size="sm" data-role="mail-save" disabled={mailSaving} onClick={doMailSave}>保存</Button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}

function FieldRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[118px_1fr] items-center gap-4 py-2">
      <span className="text-[13px] font-extrabold">{label}</span>
      {children}
    </div>
  )
}

function StatCell({ n, label, tone }: { n: number; label: string; tone?: string }) {
  return (
    <div className="px-4 py-5 max-md:py-4 [&+&]:border-l-2 [&+&]:border-foreground">
      <div className={cn('text-4xl font-extrabold tabular-nums leading-none tracking-tight max-md:text-3xl', tone)}>{n}</div>
      <div className="mt-1.5 text-xs font-semibold text-muted-foreground">{label}</div>
    </div>
  )
}

function relTime(iso?: string | null): string {
  if (!iso) return ''
  const s = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000))
  if (s < 60) return '刚刚'
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  return `${Math.floor(s / 86400)} 天前`
}
