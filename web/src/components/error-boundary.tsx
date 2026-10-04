import { Component, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

// 全局错误边界：任一页面渲染异常降级为可恢复卡片，不白屏
export class ErrorBoundary extends Component<{ children: ReactNode }, { err: Error | null }> {
  state = { err: null as Error | null }
  static getDerivedStateFromError(err: Error) {
    return { err }
  }
  render() {
    if (this.state.err) {
      return (
        <div className="mx-auto max-w-lg rounded-xl border-3 border-foreground bg-card p-6 shadow-[4px_4px_0px_var(--shadow-color)]">
          <h1 className="font-extrabold">页面出错了</h1>
          <p className="mt-2 break-all font-mono text-xs text-muted-foreground">{this.state.err.message}</p>
          <div className="mt-4 flex gap-2">
            <Button size="sm" onClick={() => location.reload()}>刷新页面</Button>
            <Button size="sm" variant="outline" onClick={() => { this.setState({ err: null }); location.assign('/') }}>回首页</Button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
