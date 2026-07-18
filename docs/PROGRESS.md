# 实施进度表（PROGRESS）

> 规则：每个关键修复完成即更新本表并独立 commit（小步提交，中断可恢复）。
> 状态：⬜ 待办 · 🔵 进行中 · ✅ 已完成 · ✔️ 验证通过（CI 绿/验收清单过）
> 本表只由主会话（总控）更新，子代理不直接改，避免并发写冲突。
> 完整规划见 [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)。

## 阶段总览

| 阶段 | 内容 | 状态 | 完成时间 |
|---|---|---|---|
| 0 | 环境准备 + 规划落库 | ✅ | 2026-07-19 |
| A | 止血：契约修复 + 可构建可部署 + CI | ✅ | 2026-07-19 |
| B | 数据层地基：goose 迁移/事务/审计/租户隔离 | ✅ | 2026-07-19 |
| C | 真实认证与 RBAC | 🔵 | |
| D | 前端基建重构 + 管理页 | ⬜ | |
| E | 入库增强 + 领用/借用/归还闭环 | ⬜ | |
| F | 维修/保养工单 + 报废 | ⬜ | |
| G | 盘点 | ⬜ | |
| H | 折旧、报表、导入导出 | ⬜ | |
| I | Webhook 接线、文档校准、CI 完整化 | ⬜ | |
| J | 终验双门禁（PM 代理 + 逻辑审计代理） | ⬜ | |

## 明细

| 阶段 | 任务 | 状态 | 提交 hash | 验证方式与结果 | 时间 |
|---|---|---|---|---|---|
| 0 | 规划写入 docs/IMPLEMENTATION_PLAN.md + 本进度表落库 | ✅ | 647089c | git commit 成功 | 2026-07-19 |
| 0 | 用户执行 docker 组授权（usermod + 重登录） | ⚠️ | | docker ps 仍 permission denied（需重新登录/newgrp），不影响 Phase A 代码修改 | 2026-07-19 |
| 0 | 安装 Go 1.25.0 → ~/.local/go | ✅ | | go version go1.25.0 linux/amd64 | 2026-07-19 |
| 0 | 安装 Node 22.19.0 LTS → ~/.local/node | ✅ | | node v22.19.0 + npm 11.16.0 | 2026-07-19 |
| 0 | 首次真实验证：`go build ./... && go test ./...` | ✅ | | 编译通过，全部 40 测试 PASS（lock/webhook 包 ok） | 2026-07-19 |
| 0 | 首次真实验证：`npm ci && npm run build` | ✅ | | tsc 0 errors + vite build ✓（107 modules, 256KB gzip 82KB） | 2026-07-19 |
| A | 前端：lifecycle 路由 404（PUT→POST /transition） | ✅ | | api.post `/transition` body `{to}` | 2026-07-19 |
| A | 前端：If-Match 引号修复（"1" → 1） | ✅ | | `String(asset.version)` 无引号 | 2026-07-19 |
| A | 前端：getApiError 统一错误提取 + 全部 catch 接入 | ✅ | | 兼容 {error:str}/{error:{message}} | 2026-07-19 |
| A | 前端：client.ts 401 直接登出（标注 TODO Phase C） | ✅ | | 注释旧 refresh 队列代码 | 2026-07-19 |
| A | 前端：Login refresh_token 兜底 'placeholder-phase-c' | ✅ | | 防 localStorage 存 undefined | 2026-07-19 |
| A | 前端：删 Agents 页+路由+导航+类型+Dashboard KPI | ✅ | | Agents.tsx 已删，NotFound 页新增 | 2026-07-19 |
| A | 前端：新增 404 兜底路由 NotFound 页 | ✅ | | `path="*"` catch-all | 2026-07-19 |
| A | 前端：模式徽标真实化（fetch /healthz 读 mode） | ✅ | | demo 时才显示绿点"演示模式" | 2026-07-19 |
| A | 前端：tsc --noEmit 通过 | ✅ | | 0 type errors | 2026-07-19 |
| A | 后端：种子 SQL bug `'$4'`→`$4` + next-tag nil check + If-Match Trim + DEMO 分页/transfer | ✅ | | 全部修完，go build 通过 | 2026-07-19 |
| A | 后端：002 迁移（system_settings + doc_sequences） | ✅ | | 新建 | 2026-07-19 |
| A | 后端：死代码删除（cache/ingest/agent/collection-agent/nginx/grafana） | ✅ | | 2,729 行死代码删除 | 2026-07-19 |
| A | 部署：Dockerfile 三阶段（node→go→alpine）+ webfs embed | ✅ | | 仓库根 Dockerfile，context=./ | 2026-07-19 |
| A | 部署：docker-compose 精简（仅 pg+app）+ .github/ci.yml | ✅ | | 删 redis/grafana，3-job CI | 2026-07-19 |
| B | Step1: 自研 migration runner（embed+EXCLUSIVE锁+schema_migrations） | ✅ | | 99 行，避免 goose 依赖链 | 2026-07-19 |
| B | Step2: 003 DROP 9 未用表 + 004 locations 表 | ✅ | | 新建迁移文件 | 2026-07-19 |
| B | Step3: DBTX 接口 + 全 repo 方法签名改造 | ✅ | | asset/assignment/user/dashboard/settings repo | 2026-07-19 |
| B | Step4: Service 层（Begin/Commit/Rollback 事务包裹) | ✅ | | asset_service(296行)+assignment_service(147行) | 2026-07-19 |
| B | Step5: IDOR 修复（全单行操作补 AND org_id=$n） | ✅ | | GetByID/Update/Delete/ForUpdate 等 | 2026-07-19 |
| B | Step6: 审计 Recorder（链式哈希 SHA256） | ✅ | | 事务内 INSERT audit_log | 2026-07-19 |
| B | Step7: org_repo + location_repo 落 PG | ✅ | | 替换内存 Store，/locations 路由注册 | 2026-07-19 |
| B | Step8: Event bus 接线（DefaultBus + log consumer） | ✅ | | service commit 后 Publish | 2026-07-19 |
| B | Step9: server.go 拆分（routes/routes_demo）+ config 清理 | ✅ | | 800+→360行；删 Redis/Vault 配置 | 2026-07-19 |
| B | go build + go test + go vet 全部通过 | ✅ | | 0 errors 0 regressions | 2026-07-19 |
