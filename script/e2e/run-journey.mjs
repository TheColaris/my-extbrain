// my-extbrain 全流程旅程 runner（轻量自研，替代 newman：需同场支持 http / cli / mcp 三通道）。
// 硬规则（承 BA 洁净室旅程纪律）：
//   1. 请求体禁止编造 ID 字面量——全部走响应变量接力（唯一直连是注册/登录拿凭据）；
//   2. 关键关联写完必须 GET 回读断言关系真实成立；
//   3. 每域至少一条负向用例。
// 用法：BASE_URL=http://127.0.0.1:8081 node run-journey.mjs [f00 f01 ...]（缺省全部，按文件名升序）
// 前置：script/e2e/e2e.sh start（洁净服务就绪）；CLI 二进制缺失时自动编译到 script/e2e/bin/extbrain。
import { readdirSync, existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { execFileSync, spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const DIR = path.dirname(fileURLToPath(import.meta.url))
const SRV = path.join(DIR, '..', '..', 'server')
const BASE = process.env.BASE_URL || 'http://127.0.0.1:8081'
const CLI_BIN = process.env.CLI_BIN || path.join(DIR, 'bin', 'extbrain')

// 全局接力环境：步骤结果与凭据都存这里（folder 间共享；经 globalThis 注入，folder 不得 import 本文件——会循环死锁）
export const S = {}
globalThis.J = S
S.BASE = BASE
S.runId8 = String(Date.now()).slice(-8) // 时间片：手机号/邮箱/路径派生唯一值，重复跑前须 reset
S.assert = (cond, msg) => { if (!cond) throw new Error(`断言失败: ${msg}`) }

// ---------- CLI 二进制 ----------
if (!existsSync(CLI_BIN)) {
  mkdirSync(path.dirname(CLI_BIN), { recursive: true })
  execFileSync('go', ['build', '-o', CLI_BIN, './cmd/extbrain'], { cwd: SRV, stdio: 'inherit' })
}
S.cliHome = (name) => {
  const h = path.join(DIR, '.cli-home', name)
  mkdirSync(h, { recursive: true })
  return h
}
S.cliReset = (name) => rmSync(path.join(DIR, '.cli-home', name), { recursive: true, force: true })
S.noteFile = path.join(DIR, '.cli-home', 'journey-note.md')
S.writeFile = (p, content) => { mkdirSync(path.dirname(p), { recursive: true }); writeFileSync(p, content) }

// ---------- mock embedding（独立子进程：避免与 execFileSync 死锁，见 mock-embed.mjs 头注）----------
const mockProc = spawn(process.execPath, [path.join(DIR, 'mock-embed.mjs')], { stdio: 'ignore' })

// ---------- 步骤执行 ----------
async function runHttp(st) {
  const headers = { 'content-type': 'application/json', ...(st.headers || {}) }
  if (st.auth) {
    const a = typeof st.auth === 'string' ? S[st.auth] : st.auth
    if (a?.authHeader) headers.authorization = a.authHeader
  }
  const p = typeof st.path === 'function' ? st.path(S) : st.path
  const body = typeof st.body === 'function' ? st.body(S) : st.body
  const res = await fetch(BASE + p, {
    method: st.method || 'GET',
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const text = await res.text()
  let d = null
  try { d = JSON.parse(text) } catch { /* 非 JSON（如 zip/md）保留 text */ }
  if (st.expect !== undefined && ![st.expect].flat().includes(res.status)) {
    throw new Error(`${st.method || 'GET'} ${p} → ${res.status}（期望 ${st.expect}）${text.slice(0, 300)}`)
  }
  S[st.name] = { status: res.status, json: d, text, headers: res.headers }
  if (st.save) for (const [k, fn] of Object.entries(st.save)) S[k] = fn(d, S, res)
  if (st.assert) st.assert(d, S, res.status, { status: res.status, text, headers: res.headers })
}

function runCli(st) {
  const home = st.home ? S.cliHome(st.home) : S.cliHome('a')
  if (st.cleanHome) rmSync(home, { recursive: true, force: true }), mkdirSync(home, { recursive: true })
  const args = typeof st.args === 'function' ? st.args(S) : st.args
  let out = ''
  try {
    out = execFileSync(CLI_BIN, args, {
      env: { ...process.env, HOME: home },
      encoding: 'utf8',
      input: st.input,
    })
  } catch (e) {
    // 允许声明非零退出码（如 logout 后 me 报未配置）
    if (st.expectCode === undefined || e.status !== st.expectCode) throw e
    out = String(e.stdout || '') + String(e.stderr || '')
  }
  S[st.name] = out
  for (const inc of st.expectOut || []) {
    if (!out.includes(inc)) throw new Error(`CLI ${args.join(' ')} 输出不含「${inc}」：${out.slice(0, 300)}`)
  }
}

async function runMcp(st) {
  const key = typeof st.key === 'string' && !st.key.startsWith('ak_live_') ? S[st.key].rawKey : st.key
  const res = await fetch(BASE + '/mcp', {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      accept: 'application/json, text/event-stream',
      authorization: 'Bearer ' + key,
    },
    body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: st.method, params: st.params || {} }),
  })
  const text = await res.text()
  let d = null
  try { d = JSON.parse(text) } catch {
    const m = text.match(/^data: (.*)$/m)
    if (m) d = JSON.parse(m[1])
  }
  if (st.expect !== undefined && ![st.expect].flat().includes(res.status)) {
    throw new Error(`MCP ${st.method} → ${res.status}（期望 ${st.expect}）${text.slice(0, 300)}`)
  }
  S[st.name] = { status: res.status, json: d, text }
  if (st.save) for (const [k, fn] of Object.entries(st.save)) S[k] = fn(d, S, res)
  if (st.assert) st.assert(d, S, res.status)
}

const RUNNERS = { http: runHttp, cli: runCli, mcp: runMcp, fn: (st) => st.fn(S), wait: (st) => new Promise((r) => setTimeout(r, st.ms)) }

// ---------- 主流程 ----------
const only = process.argv.slice(2)
const files = readdirSync(path.join(DIR, 'journey-folders'))
  .filter((f) => f.endsWith('.mjs') && (only.length === 0 || only.some((o) => f.startsWith(o))))
  .sort()

let pass = 0
const t0 = Date.now()
try {
  for (const f of files) {
    const mod = await import(path.join(DIR, 'journey-folders', f))
    const steps = mod.steps || mod.default
    console.log(`\n━━━ ${f.replace('.mjs', '')} · ${mod.meta || ''}（${steps.length} 步）`)
    for (const st of steps) {
      try {
        await RUNNERS[st.t](st)
        pass++
        if (process.env.VERBOSE) console.log(`  ✓ ${st.name}`)
      } catch (e) {
        console.error(`  ✗ [${f}] ${st.name}: ${e.message}`)
        throw e
      }
    }
    console.log(`  ✓ ${f.replace('.mjs', '')} 全绿`)
  }
} finally {
  mockProc.kill('SIGTERM')
}
console.log(`\n═══ 旅程完成：${pass} 步全绿 · ${files.length} 域 · ${((Date.now() - t0) / 1000).toFixed(1)}s ═══`)
