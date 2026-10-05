# E2E 洁净室 + 全流程旅程

> 一条命令从零跑完「注册 → 待办/便签 → 知识库 → 仓库/目录权限 → 混合检索 → 分享 → 回收站 → 导出 → 通知推送 → 平台管理 → CLI 设备授权 → CLI 真二进制 → MCP → 审计对账」的真实主旅程。
> 机制承 BA（beat-array）洁净室体系：三重门 fail-closed、外发截获、变量接力旅程。

## 快速使用

```bash
script/e2e/e2e.sh up        # 起洁净库容器（my-extbrain-pg-e2e，宿主 5434，独立于 dev 5433）
script/e2e/e2e.sh reset     # 重建洁净库 extbrain_e2e（stop 服务 → DROP/CREATE）
script/e2e/e2e.sh start     # 起洁净服务 :8081（bootstrap 建表 → 写门③标记行 → 带三重门重启）
script/e2e/e2e.sh journey   # reset + start + 全旅程（失败保留现场，成功自动 stop）
script/e2e/e2e.sh stop|down # 停服务 / 停服务+停容器（数据卷保留）
script/e2e/e2e.sh destroy   # 销毁：删容器+删数据卷（不可逆；独立命令，与 journey/单测无关，须显式单独执行）
```

单独跑旅程（服务已在跑时）：`BASE_URL=http://127.0.0.1:8081 node script/e2e/run-journey.mjs [f00 f01 ...]`

## 三重门（fail-closed，`server/internal/e2e/`）

1. **门① 意图门**：环境变量 `E2E_MODE=true`；
2. **门② 配置门**：`DATABASE_URL` 库名必须是 `extbrain_e2e`（防配置漂移指到 dev/生产库）；
3. **门③ 数据门**：库内存在标记行 `tp_system_config.e2e_cleanroom_marker`（`e2e.sh start` 的 seed 段写入）。

`E2E_MODE=true` 但任一门不过 → 启动直接报错退出（`[E2E][启动断言失败]`）；门不开时信箱 API 404、捕获分支全部旁路。三道门各覆盖单测（`internal/e2e/e2e_test.go`）。

## 外发截获（信箱）

| 外部依赖 | 边界 | E2E 模式行为 |
| --- | --- | --- |
| 钉钉/飞书 webhook | `notify_service.postJSON`（唯一出站口） | 不外发；消息体落 `tl_e2e_mailbox`，按成功返回 |
| 浏览器推送 | `push_service.webPush` | 不真发；payload 落信箱 |
| embedding | 不改代码 | 洁净配方指向 mock 端点（`mock-embed.mjs`，按关键词分簇的确定性向量，旅程自动起停） |

信箱 API（仅门全开注册；`GET/DELETE /api/v1/e2e/mailbox?channel=&target=`）：旅程用它断言「到期提醒真的推到了钉钉/飞书/浏览器」。**E2E 模式下绝不回落真实外发。**

## 旅程纪律（`run-journey.mjs` + `journey-folders/`）

1. **禁止编造 ID 字面量**——全部走响应变量接力（`S` 环境；唯一直连是注册/登录拿凭据）；
2. **关键关联写完必须回读断言**（Key 列表打码、跨仓库移动后新路径 GET、MCP 写后 REST 读……）；
3. **每域至少一条负向**（重复注册/scope 越权/路径穿越/他人数据/错 token/重放授权码……）；
4. 运行时唯一值由 `runId8`（时间片）派生，重复执行前必须 reset（`journey` 聚合入口已保证）；
5. 文件夹按域独立（f00-f20），可单独跑任意子集。

## 已知行为对齐（旅程断言以现实为准）

- PUT notes 是 upsert 语义，恒 200（无 201）；CLI/MCP 的 note 域 key_ids 为数字数组；`is_pinned` 是 int16；
- `/search` 缺省即三域结构 `{todos, memos, notes}`（CLI searchCmd 按此解析——曾因服务端 v2 改三域导致 CLI 恒 0 条，旅程抓出后已修）；
- 公开限流在洁净模式放宽到 200/min（`main.go e2ePubLimit`）——旅程多轮快跑不撞限流，限流逻辑自身由单测保证；
- 审计异步落库（1s flush）：来源反查类断言前需 wait；浏览计数为同步自增但 owner 视角才可见。
