import { useEffect, useRef, useState } from 'react'
import QRCode from 'qrcode'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { undoToast } from '@/components/undo-toast'
import { ApiError, sharesApi, type NoteFull, type ShareItem } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { SHARE_MODE, type ShareMode } from '@/lib/enums'
import { cn } from '@/lib/utils'

// 笔记分享弹窗：无活跃分享=生成设置（模式/有效期）；有=三 Tab（链接/二维码/图片）。
// 链接=location.origin+/share/<token>（游客可读）；二维码/图片纯前端 canvas 生成。
// 一篇笔记一个活跃分享：再生成=保留 token 更新设置并重冻快照；停止分享后再生成=新 token。

const EXPIRE_OPTS: { days: number; label: string }[] = [
  { days: 1, label: '1 天' },
  { days: 7, label: '7 天' },
  { days: 30, label: '30 天' },
  { days: 0, label: '永久' },
]
const MODE_OPTS: { mode: ShareMode; label: string; hint: string }[] = [
  { mode: SHARE_MODE.snapshot, label: '快照冻结', hint: '按当前内容定格，之后的改动不影响分享' },
  { mode: SHARE_MODE.live, label: '跟随更新', hint: '别人打开始终看到最新版；删除原笔记即失效' },
]

// 主题色（对齐 index.css light 主题；canvas 拿不到 CSS 变量）
const INK = '#1c1c1e'
const CREAM = '#fff8e5'
const LEMON = '#ffe566'

export function ShareDialog({ open, onOpenChange, path, note }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  path: string
  note: NoteFull | null
}) {
  const [phase, setPhase] = useState<'loading' | 'none' | 'share'>('loading')
  const [share, setShare] = useState<ShareItem | null>(null)
  const [mode, setMode] = useState<ShareMode>(SHARE_MODE.snapshot)
  const [days, setDays] = useState(30)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    let alive = true
    setPhase('loading')
    setShare(null)
    sharesApi
      .get(path)
      .then((r) => {
        if (!alive) return
        setShare(r.share)
        setPhase(r.share ? 'share' : 'none')
      })
      .catch(() => { if (alive) setPhase('none') })
    return () => { alive = false }
  }, [open, path])

  const url = share ? `${location.origin}/share/${share.token}` : ''

  const create = async () => {
    setBusy(true)
    try {
      const r = await sharesApi.create(path, mode, days)
      setShare(r.share)
      setPhase('share')
      undoToast('分享链接已生成')
    } catch (e) {
      undoToast(e instanceof ApiError ? e.message : '生成失败')
    } finally {
      setBusy(false)
    }
  }

  const revoke = async () => {
    if (!share) return
    setBusy(true)
    try {
      await sharesApi.revoke(share.token)
      setShare(null)
      setPhase('none')
      undoToast('已停止分享，链接即刻失效')
    } catch (e) {
      undoToast(e instanceof ApiError ? e.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-role="dlg-share" className="max-w-[560px]">
        <DialogHeader><DialogTitle>分享笔记</DialogTitle></DialogHeader>

        {phase === 'loading' && (
          <div className="py-8 text-center"><span className="bk-loader" style={{ width: 20, height: 20, borderWidth: 4 }} /></div>
        )}

        {phase === 'none' && (
          <>
            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label>分享内容</Label>
                <div className="truncate border-2 border-foreground bg-background px-3 py-2 font-mono text-xs font-semibold">{note?.title ?? path}</div>
              </div>
              <div className="space-y-1.5">
                <Label>模式</Label>
                <div className="flex border-3 border-foreground shadow-[3px_3px_0px_var(--shadow-color)]">
                  {MODE_OPTS.map((o, i) => (
                    <button
                      key={o.mode} type="button"
                      className={cn('flex-1 cursor-pointer py-1.5 text-[13px] font-bold', i > 0 && 'border-l-3 border-foreground', o.mode === mode ? 'bg-primary' : 'bg-background')}
                      onClick={() => setMode(o.mode)}
                    >{o.label}</button>
                  ))}
                </div>
                <p className="text-[11.5px] font-semibold text-muted-foreground">{MODE_OPTS.find((o) => o.mode === mode)?.hint}</p>
              </div>
              <div className="space-y-1.5">
                <Label>有效期</Label>
                <div className="flex border-3 border-foreground shadow-[3px_3px_0px_var(--shadow-color)]">
                  {EXPIRE_OPTS.map((o, i) => (
                    <button
                      key={o.days} type="button"
                      className={cn('flex-1 cursor-pointer py-1.5 text-[13px] font-bold', i > 0 && 'border-l-3 border-foreground', o.days === days ? 'bg-primary' : 'bg-background')}
                      onClick={() => setDays(o.days)}
                    >{o.label}</button>
                  ))}
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
              <Button data-role="share-create" disabled={busy || !note} onClick={create}>{busy ? <span className="bk-loader" /> : '生成分享链接'}</Button>
            </DialogFooter>
          </>
        )}

        {phase === 'share' && share && (
          <>
            <Tabs defaultValue="link">
              <TabsList className="w-full">
                <TabsTrigger value="link" className="flex-1">链接</TabsTrigger>
                <TabsTrigger value="qr" className="flex-1">二维码</TabsTrigger>
                <TabsTrigger value="img" className="flex-1">图片</TabsTrigger>
              </TabsList>
              <TabsContent value="link" className="space-y-3">
                <div className="flex gap-2">
                  <Input data-role="share-url" readOnly value={url} onFocus={(e) => e.target.select()} className="font-mono text-xs" />
                  <Button data-role="share-copy" variant="outline" className="shrink-0" onClick={async () => { if (await copyText(url)) undoToast('链接已复制') }}>复制</Button>
                </div>
                <div className="flex flex-wrap items-center gap-2 text-[11.5px] font-semibold text-muted-foreground">
                  <span className="rounded border-2 border-foreground bg-card px-1.5 py-0.5 font-bold text-foreground">{share.mode === SHARE_MODE.live ? '跟随更新' : '快照冻结'}</span>
                  <span>{fmtExpire(share.expire_time)}</span>
                  <span>·</span>
                  <span>被看 {share.view_count} 次</span>
                </div>
                <p className="text-[11.5px] font-semibold text-muted-foreground">链接无需登录即可查看；停止分享后立刻失效。</p>
              </TabsContent>
              <TabsContent value="qr" className="space-y-3">
                <div className="flex justify-center border-3 border-foreground bg-card p-4 shadow-[4px_4px_0px_var(--shadow-color)]">
                  <QRCanvas url={url} size={216} />
                </div>
                <div className="flex justify-center">
                  <Button data-role="share-qr-dl" variant="outline" onClick={() => downloadCanvas('qr', `外脑分享二维码-${share.title || '笔记'}.png`)}>下载二维码</Button>
                </div>
              </TabsContent>
              <TabsContent value="img" className="space-y-3">
                <div className="border-3 border-foreground bg-card p-3 shadow-[4px_4px_0px_var(--shadow-color)]">
                  <ShareCardCanvas note={note} url={url} expire={share.expire_time} />
                </div>
                <div className="flex justify-center">
                  <Button data-role="share-img-dl" onClick={() => downloadCanvas('card', `外脑分享-${(note?.title || '笔记').slice(0, 20)}.png`)}>下载图片</Button>
                </div>
              </TabsContent>
            </Tabs>
            <DialogFooter>
              <Button variant="destructive" className="mr-auto" disabled={busy} onClick={revoke}>停止分享</Button>
              <Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function fmtExpire(iso?: string): string {
  if (!iso) return '永久有效'
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} 到期`
}

/* ===== 二维码（data-canvas=qr 供下载定位） ===== */
function QRCanvas({ url, size }: { url: string; size: number }) {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    if (!ref.current) return
    QRCode.toCanvas(ref.current, url, {
      width: size, margin: 2,
      color: { dark: INK, light: '#ffffff' },
      errorCorrectionLevel: 'M',
    }).catch(() => undoToast('二维码生成失败'))
  }, [url, size])
  return <canvas ref={ref} data-canvas="qr" style={{ display: 'block' }} />
}

/* ===== 分享卡图片：品牌头 + 标题/标签 + 内容预览 + 二维码底条（neubrutalism 配色，2x 高清） ===== */
const CARD_W = 750
const DPR = 2
const PAD = 40 // 卡内主边距（1x 逻辑坐标）
const TEXT_W = CARD_W - PAD * 2 - 8 // 文本可用宽

export function drawShareCard(cv: HTMLCanvasElement | null, note: NoteFull | null, url: string, expire?: string) {
  if (!cv || !note) return
  const zh = (w: string, px: number) => `${w} ${px}px "PingFang SC","Microsoft YaHei",system-ui,sans-serif`
  const mo = (w: string, px: number) => `${w} ${px}px ui-monospace,"SF Mono",Menlo,monospace`
  // 统一 1x 逻辑坐标排版（measure 用临时 ctx），渲染时 scale(DPR)
  const mc = document.createElement('canvas').getContext('2d')!
  const widthOf = (text: string, weight: string, px: number, isMono = false) => {
    mc.font = isMono ? mo(weight, px) : zh(weight, px)
    return mc.measureText(text).width
  }
  const wrap = (text: string, weight: string, px: number, maxW = TEXT_W): string[] => {
    const lines: string[] = []
    let cur = ''
    for (const ch of text) {
      if (cur && widthOf(cur + ch, weight, px) > maxW) { lines.push(cur); cur = ch }
      else cur += ch
    }
    if (cur) lines.push(cur)
    return lines.length ? lines : ['']
  }

  const titlePx = 30, bodyPx = 17, lineH = 30, headH = 88, footH = 176, maxBodyLines = 14
  const titleLines = wrap(note.title || note.path, '800', titlePx).slice(0, 2)
  const allBody: string[] = []
  for (const ln of plainMdLines(note.content)) allBody.push(...wrap(ln, '400', bodyPx))
  const truncated = allBody.length > maxBodyLines
  const bodyShown = allBody.slice(0, maxBodyLines)
  const tagsH = (note.tags?.length ?? 0) > 0 ? 42 : 0

  const H = Math.round(
    PAD + headH + 30 + titleLines.length * 44 + 10 + tagsH + 16 +
    bodyShown.length * lineH + (truncated ? 36 : 8) + 26 + footH + PAD,
  )

  cv.width = CARD_W * DPR
  cv.height = H * DPR
  const x = cv.getContext('2d')!
  x.scale(DPR, DPR)

  // 卡底：奶油 + 圆角黑框 + 右下硬阴影
  x.fillStyle = 'rgba(0,0,0,.10)'
  roundRect(x, PAD * 0.3 + 10, PAD * 0.3 + 10, CARD_W - PAD * 0.6, H - PAD * 0.6, 18)
  x.fill()
  x.fillStyle = CREAM
  roundRect(x, PAD * 0.3, PAD * 0.3, CARD_W - PAD * 0.6, H - PAD * 0.6, 18)
  x.fill()
  x.lineWidth = 4; x.strokeStyle = INK
  roundRect(x, PAD * 0.3, PAD * 0.3, CARD_W - PAD * 0.6, H - PAD * 0.6, 18)
  x.stroke()

  // 品牌头：黄条 + 脑图 logo + 站名
  x.fillStyle = LEMON
  x.fillRect(PAD, PAD, CARD_W - PAD * 2, headH)
  x.lineWidth = 3; x.strokeStyle = INK
  x.strokeRect(PAD, PAD, CARD_W - PAD * 2, headH)
  drawLogo(x, PAD + 26, PAD + headH / 2)
  x.fillStyle = INK
  x.font = zh('800', 24); x.textBaseline = 'middle'
  x.fillText('我的外脑', PAD + 80, PAD + headH / 2 - 8)
  x.font = mo('600', 13)
  x.fillStyle = 'rgba(28,28,30,.65)'
  x.fillText('my-extbrain', PAD + 80, PAD + headH / 2 + 18)

  // 标题
  let y = PAD + headH + 30 + titlePx
  x.textBaseline = 'alphabetic'
  x.fillStyle = INK
  x.font = zh('800', titlePx)
  for (const ln of titleLines) { x.fillText(ln, PAD, y); y += 44 }

  // 标签 chip
  if (tagsH) {
    y += 8
    x.font = zh('700', 14)
    let tx = PAD
    for (const t of note.tags!.slice(0, 5)) {
      const w = x.measureText(t).width + 20
      x.fillStyle = '#ffffff'
      x.fillRect(tx, y - 14, w, 26)
      x.lineWidth = 2; x.strokeStyle = INK; x.strokeRect(tx, y - 14, w, 26)
      x.fillStyle = INK; x.fillText(t, tx + 10, y)
      tx += w + 10
    }
    y += 34
  }

  // 正文预览
  y += 16
  x.font = zh('400', bodyPx)
  x.fillStyle = '#3a3a3e'
  for (const ln of bodyShown) { x.fillText(ln, PAD, y); y += lineH }
  if (truncated) {
    x.font = zh('700', 15)
    x.fillStyle = INK
    x.fillText('…… 内容有省略，扫码读全文', PAD, y + 2)
  }

  // 底条：二维码 + 引导文案
  const fy = H - PAD - footH
  x.fillStyle = '#ffffff'
  x.fillRect(PAD, fy, CARD_W - PAD * 2, footH)
  x.lineWidth = 3; x.strokeStyle = INK
  x.strokeRect(PAD, fy, CARD_W - PAD * 2, footH)
  const qrSize = footH - 44
  const qc = document.createElement('canvas')
  void QRCode.toCanvas(qc, url, {
    width: qrSize * DPR, margin: 1,
    color: { dark: INK, light: '#ffffff' }, errorCorrectionLevel: 'M',
  }).then(() => { x.drawImage(qc, PAD + 22, fy + 22, qrSize, qrSize) })
  const rx = PAD + 22 + qrSize + 26
  x.fillStyle = INK
  x.font = zh('800', 22)
  x.fillText('扫码阅读全文', rx, fy + 60)
  x.font = mo('600', 13)
  x.fillStyle = '#3a3a3e'
  x.fillText(url.replace(/^https?:\/\//, ''), rx, fy + 90)
  x.font = zh('600', 13)
  x.fillStyle = '#6b6b70'
  x.fillText(expire ? `分享至 ${new Date(expire).toLocaleDateString('zh-CN')} · 过期失效` : '链接长期有效 · 可随时停止', rx, fy + 116)
}

function ShareCardCanvas({ note, url, expire }: { note: NoteFull | null; url: string; expire?: string }) {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => { drawShareCard(ref.current, note, url, expire) }, [note, url, expire])
  return <canvas ref={ref} data-canvas="card" style={{ display: 'block', width: '100%' }} />
}

/** MD 轻度纯化：去常见标记（代码块整块保留缩进形态） */
export function plainMdLines(md: string): string[] {
  const out: string[] = []
  let inCode = false
  for (const raw of md.split('\n')) {
    const line = raw.replace(/\s+$/, '')
    if (line.startsWith('```')) { inCode = !inCode; continue }
    if (inCode) { out.push('  ' + line); continue }
    const s = line
      .replace(/^#{1,6}\s+/, '')
      .replace(/^>\s?/, '')
      .replace(/^[-*+]\s+/, '· ')
      .replace(/^\d+[.)]\s+/, (m) => m.trim() + ' ')
      .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/(\*\*|__)(.*?)\1/g, '$2')
      .replace(/(\*|_)(.*?)\1/g, '$2')
      .replace(/`([^`]*)`/g, '$1')
      .replace(/^(-{3,}|\*{3,})$/, '———')
    if (s.trim() === '') continue
    out.push(s)
  }
  return out
}

function drawLogo(x: CanvasRenderingContext2D, cx: number, cy: number) {
  x.save()
  x.translate(cx, cy)
  x.lineWidth = 2.2; x.strokeStyle = INK
  const node = (nx: number, ny: number, r: number) => { x.beginPath(); x.arc(nx, ny, r, 0, Math.PI * 2); x.stroke() }
  const link = (x1: number, y1: number, x2: number, y2: number) => { x.beginPath(); x.moveTo(x1, y1); x.lineTo(x2, y2); x.stroke() }
  node(-14, -8, 4); node(6, -11, 4); node(0, 0, 6); node(-13, 8, 3.5); node(13, 8, 3.5)
  link(-10, -6, -4, -3); link(4, -8, 1, -4); link(-4, 3, -10, 6); link(4, 3, 10, 6)
  x.restore()
}

function roundRect(x: CanvasRenderingContext2D, a: number, b: number, w: number, h: number, r: number) {
  x.beginPath()
  x.moveTo(a + r, b)
  x.arcTo(a + w, b, a + w, b + h, r)
  x.arcTo(a + w, b + h, a, b + h, r)
  x.arcTo(a, b + h, a, b, r)
  x.arcTo(a, b, a + w, b, r)
  x.closePath()
}

function downloadCanvas(kind: 'qr' | 'card', filename: string) {
  const cv = document.querySelector<HTMLCanvasElement>(`canvas[data-canvas="${kind}"]`)
  if (!cv) { undoToast('图片尚未生成'); return }
  const a = document.createElement('a')
  a.href = cv.toDataURL('image/png')
  a.download = filename
  a.click()
}
