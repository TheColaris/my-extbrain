# my-extbrain · 协作规范

面向贡献者与 AI 编码助手（AGENTS.md）的仓库协作规范。

## 仓库结构

| 位置 | 内容 |
|---|---|
| `server/` | Go 后端 + CLI（Gin + GORM + golang-migrate + skill/），前端产物构建时 embed 进单二进制 |
| `web/` | React 19 + Vite + TS + BoldKit（neubrutalism） |

## 硬规则

1. **UI 一律用 BoldKit 原生组件**（boldkit.dev，React 版；shadcn CLI 不可用时走 registry 直拉），禁止手写复刻组件。
2. **枚举优先，禁止魔法值**：唯一真源 = `server/internal/model/enums.go`；web 侧 `src/lib/enums.ts` 对齐同一组字符串值，新增枚举两处同步。
3. **建表按统一规范**：前缀 td_/tf_/tp_/tr_/tu_/tl_/ts_、必备字段（id/create_time/update_time）、pk_/uk_/idx_ 索引命名、无物理外键、超 5000 长度 text 独立表；PG 侧用 timestamptz/smallint/text[]。
4. **设计对齐**：UI 实现须与设计稿 1:1 对齐，冲突先改设计再改代码（设计稿本地维护，不入库）。
5. **版本**：单仓统一打 tag（v0.x.x），server/web 同版本号；构建链 = web dist → server embed。

## 开发

```bash
cd web && pnpm install && pnpm dev      # 前端（/api 代理到 127.0.0.1:8080）
cd server && go test ./...              # 后端测试
```
