// mock embedding server（独立进程）：OpenAI 兼容 /embeddings，按关键词分簇的确定性向量。
// 必须独立于旅程进程——CLI 步骤用 execFileSync 同步阻塞事件循环，mock 若同进程会死锁
// （CLI 等 server 响应 → server 等 mock → mock 被阻塞）。
import http from 'node:http'

// 簇设计（对齐 server 侧 vector_search_test 的 fake 思路）：0=限流族 1=docker 族 2=空簇（无笔记）3=其余
function fakeEmbed(text) {
  const v = new Array(1024).fill(0.01)
  let c = 3
  if (/限流|ratelimit/i.test(text)) c = 0
  else if (/docker|容器/i.test(text)) c = 1
  else if (/quantumflux/i.test(text)) c = 2
  v[c] = 1
  return v
}

const hits = []
const srv = http.createServer((req, res) => {
  let b = ''
  req.on('data', (ch) => (b += ch))
  req.on('end', () => {
    try {
      const input = JSON.parse(b).input || []
      hits.push(...input)
      res.writeHead(200, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ data: input.map((t) => ({ embedding: fakeEmbed(String(t)) })) }))
    } catch (e) {
      res.writeHead(400).end(JSON.stringify({ error: { message: String(e) } }))
    }
  })
})
srv.listen(8199, '127.0.0.1', () => console.log('mock embedding ready'))
process.on('SIGTERM', () => process.exit(0))
