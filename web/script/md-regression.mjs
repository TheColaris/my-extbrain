// Markdown 渲染回归：自动链接截断（全角标点/CJK 不进 href）+ 结构渲染抽查。
// 运行：cd web && node script/md-regression.mjs（Node 24 原生类型擦除，无需构建）
import assert from 'node:assert/strict'
import { marked } from 'marked'
import '../src/lib/md.ts' // 副作用：安装自动链接截断

let failed = 0
const check = (name, fn) => {
  try {
    fn()
    console.log(`  ✓ ${name}`)
  } catch (e) {
    failed++
    console.error(`  ✗ ${name}\n    ${e.message}`)
  }
}
const inline = (s) => marked.parseInline(s, { async: false })

console.log('自动链接截断：')
check('全角逗号 + 中文说明不吞进链接', () => {
  const html = inline('见 https://gateway.ai.cloudflare.com/v1/x/gw/compat，客户端自己拼')
  assert.match(html, /<a href="https:\/\/gateway\.ai\.cloudflare\.com\/v1\/x\/gw\/compat">/)
  assert.ok(!html.includes('%EF%BC%8C'), `href 不应含全角逗号编码：${html}`)
  assert.ok(html.includes('</a>，客户端自己拼'), `中文应留在链接外：${html}`)
})
check('句号/分号/右括号/顿号边界', () => {
  for (const tail of ['。', '；', '）', '、']) {
    const html = inline(`x https://a.com/b${tail}后文`)
    assert.ok(html.includes('<a href="https://a.com/b">'), `${tail} 应截断（got ${html}）`)
  }
})
check('邮箱与 www 保持内置行为', () => {
  assert.match(inline('联系 foo@bar.com，谢谢'), /<a href="mailto:foo@bar\.com">/)
  assert.match(inline('www.example.com，还有'), /<a href="http:\/\/www\.example\.com">www\.example\.com<\/a>，还有/)
})
check('查询串/连字符/下划线保留', () => {
  const html = inline('看 https://a.com/x?a=1&b=2-y_z，后面')
  assert.match(html, /<a href="https:\/\/a\.com\/x\?a=1&amp;b=2-y_z">/)
})
check('ASCII 标点结尾仍按内置回退', () => {
  assert.match(inline('见 https://a.com/x.'), /<a href="https:\/\/a\.com\/x">https:\/\/a\.com\/x<\/a>\./)
})
check('无空格纯 URL 结尾不截断', () => {
  assert.match(inline('见 https://a.com/x'), /<a href="https:\/\/a\.com\/x">/)
})

console.log('结构渲染：')
check('标题/列表/代码块/表格/引用齐备', () => {
  const html = marked.parse('# 一\n\n- a\n- b\n\n```\ncode\n```\n\n| a |\n|---|\n| 1 |\n\n> q\n', { async: false, breaks: true })
  for (const tag of ['<h1>', '<ul>', '<pre>', '<table>', '<blockquote>']) assert.ok(html.includes(tag), `缺 ${tag}`)
})
check('breaks: true 单换行成 <br>', () => {
  assert.match(marked.parse('一行\n二行', { async: false, breaks: true }), /<br>/)
})

if (failed) {
  console.error(`\n${failed} 项失败`)
  process.exit(1)
}
console.log('\n全部通过')
