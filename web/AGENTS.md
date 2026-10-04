# extbrain-web AGENTS

React 前端子仓。硬规则（真源=根目录 AGENTS.md）：

1. 组件只用 BoldKit 原生（`src/components/ui/`，registry 安装配方见 README）；缺组件先装，不手写复刻。
2. 枚举唯一真源在 server `internal/model/enums.go`；本仓 `src/lib/enums.ts` 同字符串值对齐，两处同步（Go/TS）。
3. 布局骨架：侧栏=品牌块/分组菜单/底部用户卡片；主区 topbar+面包屑；PageContainer 统一宽度。
4. API 走相对路径 `/api/v1`（dev 由 vite proxy 转 8080，生产同源 embed）。
5. 样式只用 Tailwind 工具类 + BoldKit 主题变量（`--shadow-color`/neon/clash 色板），不引外部 CSS。
