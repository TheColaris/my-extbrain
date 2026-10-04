/**
 * 通用组件：撤销 Toast —— 页面无关，任何需要「操作可撤销」的列表页共用。
 * 口径：单实例（新 toast 顶掉旧的，不做堆叠）、
 * 默认 5.2s 自愈、点动作先执行回调再消失。todos 的 完成/删除/清空 与
 * 便签/知识库的删除撤销都走本组件，禁止页面内再散落 sonner 副本。
 *
 * 用法：
 *   undoToast('已记入')
 *   undoToast('已完成', { label: '撤销', onAction: async () => { … } })
 */
import { toast } from 'sonner'

/** 固定 id ⇒ sonner 同 id 替换语义 = 单实例 */
const UNDO_TOAST_ID = 'undo-toast'

export function undoToast(
  message: string,
  opts?: { label?: string; onAction?: () => void | Promise<void>; duration?: number }
) {
  toast(message, {
    id: UNDO_TOAST_ID,
    duration: opts?.duration ?? 5200,
    ...(opts?.label
      ? { action: { label: opts.label, onClick: () => { void opts.onAction?.() } } }
      : {}),
  })
}
