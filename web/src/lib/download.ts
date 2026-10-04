// 下载工具：前端 Blob 生成 .md 文件（内容已在内存，零请求）。
// 文件名清洗：路径分隔符与 Windows 非法字符替换为 -，缺 .md 自动补。
export function downloadMd(filename: string, content: string) {
  const safe = filename.replace(/[/\\:*?"<>|\n\r]+/g, '-').trim() || '笔记'
  const name = /\.md$/i.test(safe) ? safe : safe + '.md'
  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
