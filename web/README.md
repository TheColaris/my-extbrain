# extbrain-web

[my-extbrain（我的外脑）](..) 的前端：React 19 + Vite + TS + Tailwind v4 + **BoldKit**（neubrutalism 组件库）。

## 开发

```bash
pnpm install
pnpm dev        # http://localhost:5173，/api 代理到 127.0.0.1:8080（server/ compose）
pnpm build      # tsc + vite build，产物 dist/（由 server 构建时 embed）
```

## BoldKit 组件安装（本机配方）

```bash
export HTTPS_PROXY=http://127.0.0.1:<你的代理端口>
B=https://boldkit.dev/r
npx -y shadcn@latest add "$B/button.json" "$B/dialog.json" ... --yes
```

- registry 直连 `https://boldkit.dev/r/<name>.json`（标准 shadcn registry 协议，全量清单 `/r/registry.json`）
- **shadcn CLI 需要代理环境变量才能拉 registry**（无代理时 fetch 失败，易误判为 CLI 故障）；CLI 完全不可用时 fallback：curl 拉 JSON 取 `files[].content` 落盘
- 主题变量在 `src/index.css`（BoldKit theme 组件注入，`--shadow-color` 等）

## 硬规则

- **只用 BoldKit 原生组件**（`src/components/ui/`），禁止手写复刻。
- 枚举：`src/lib/enums.ts` 与 server `internal/model/enums.go` 同值对齐，禁止魔法值。
- 界面 1:1 对齐设计稿，冲突先改设计再改代码。
