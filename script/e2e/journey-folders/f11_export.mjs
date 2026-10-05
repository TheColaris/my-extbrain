const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f11 整体导出：zip 结构与完整性
export const meta = '整体导出'
export const steps = [
  { t: 'http', name: 'exportZip', path: '/api/v1/export', auth: 'A', expect: 200, assert: (d, S, st4, st) => {
      S.assert(st.text.length > 100, '导出应非空')
      // zip 魔数 PK\x03\x04（text 以 latin1 读出前两字节应为 P K）
      S.assert(st.text.charCodeAt(0) === 0x50 && st.text.charCodeAt(1) === 0x4b, '响应应为 zip 魔数开头') } },
  { t: 'http', name: 'exportNoAuth', path: '/api/v1/export', expect: 401 },
]