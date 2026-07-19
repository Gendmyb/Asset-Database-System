# API 参考

Base: `/api/v1`。Auth: `Bearer <access_token>`（Header）。15min TTL，过期用 `/auth/refresh` 续期。

## 认证

| 方法 | 路径 | Auth | 说明 |
|---|---|---|---|
| POST | /auth/login | 无 | `{username, password}` → `{access_token, refresh_token, user}` |
| POST | /auth/refresh | 无 | `{refresh_token}` → 新 token 对 |
| POST | /auth/logout | 无 | `{refresh_token}` → 吊销全 family |
| GET | /me | Bearer | 当前用户信息 |
| PUT | /me/password | Bearer | `{old_password, new_password}` |

## 资产

| 方法 | 路径 | RBAC | 说明 |
|---|---|---|---|
| GET | /assets | viewer+ | 搜索（?search=name&type=SN）、筛选（status/lifecycle）、游标分页 |
| POST | /assets | manager+ | 创建（含采购字段 + 折旧参数），编号空=自动生成 |
| POST | /assets/batch | manager+ | `{...asset_fields, count: N}` 事务内批量创建 |
| GET | /assets/:id | viewer+ | 详情 |
| PUT | /assets/:id | manager+ | 更新（需 If-Match version 头） |
| DELETE | /assets/:id | manager+ | 软删除 |
| POST | /assets/:id/transition | manager+ | `{to}` 生命周期转换 |
| POST | /assets/:id/retire | admin+ | `{reason}` 报废终态 |
| GET | /assets/:id/history | viewer+ | 审计日志 |

## 领用/借用

| 方法 | 路径 | RBAC | 说明 |
|---|---|---|---|
| POST | /assets/:id/assign | manager+ | `{assigned_to, notes?}` 长期领用 |
| POST | /assets/:id/borrow | manager+ | `{assigned_to, due_date(必填), notes?}` 临时借用 |
| POST | /assets/:id/release | manager+ | `{return_notes?}` 归还 |
| POST | /assets/:id/transfer | manager+ | `{assigned_to, notes?}` 转移（借用不可转） |
| GET | /assets/:id/assignments | viewer+ | 当前活跃记录 |
| GET | /assignments | viewer+ | 列表（?status/type/assigned_to/overdue/cursor） |
| GET | /users/:id/assignments | viewer+ | 某用户记录 |

## 维修工单

| 方法 | 路径 | RBAC |
|---|---|---|
| POST | /maintenance-orders | manager+ |
| GET | /maintenance-orders | viewer+ |
| GET | /maintenance-orders/:id | viewer+ |
| POST | /maintenance-orders/:id/start | manager+ |
| POST | /maintenance-orders/:id/complete | manager+ |
| POST | /maintenance-orders/:id/cancel | manager+ |

## 盘点

| 方法 | 路径 | RBAC | 说明 |
|---|---|---|---|
| POST | /stocktakes | admin+ | 创建计划 |
| GET | /stocktakes | viewer+ | 列表 |
| GET | /stocktakes/:id | viewer+ | 详情（含进度统计） |
| POST | /stocktakes/:id/start | admin+ | 快照生成 items |
| PUT | /stocktakes/:id/items/:itemId | manager+ | 逐项核对 `{result, actual_location_id?, notes?}` |
| POST | /stocktakes/:id/items | manager+ | 盘盈登记 `{surplus_note}` |
| POST | /stocktakes/:id/complete | admin+ | `{apply_moves?: bool}` |
| GET | /stocktakes/:id/report | viewer+ | 报告（?format=csv） |

## 报表

| 方法 | 路径 | RBAC |
|---|---|---|
| GET | /reports/summary | viewer+ |
| GET | /reports/depreciation | viewer+ |
| GET | /reports/maintenance-cost | viewer+ |
| GET | /reports/assignments-due | viewer+ |

## 导入导出

| 方法 | 路径 | RBAC | 说明 |
|---|---|---|---|
| GET | /assets/export | admin+ | CSV（?format=csv，透传筛选参数） |
| GET | /assets/import/template | manager+ | CSV 模板 |
| POST | /assets/import | manager+ | multipart file（?dry_run=true 预检） |

## 基础数据

| 方法 | 路径 | RBAC | 说明 |
|---|---|---|---|
| GET | /asset-types | viewer+ | 资产类型列表 |
| GET | /users | viewer+ | 用户列表 |
| GET/PUT | /settings | admin+ | 系统设置 |
| GET | /settings/next-tag | viewer+ | 下一个编号 |
| GET | /locations | viewer+ | 位置列表 |
| POST/PUT/DELETE | /locations, /locations/:id | admin+ | 位置增删改 |

## Webhook

| 方法 | 路径 | RBAC |
|---|---|---|
| GET | /admin/webhooks | admin+ |
| POST | /admin/webhooks | admin+ |
| GET/PUT/DELETE | /admin/webhooks/:id | admin+ |
| GET | /admin/webhooks/:id/deliveries | admin+ |

## 用户管理

| 方法 | 路径 | RBAC |
|---|---|---|
| GET | /admin/users | admin+ |
| POST | /admin/users | admin+ |
| PUT | /admin/users/:id | admin+ |
| POST | /admin/users/:id/reset-password | admin+ |

## 统一响应格式

```json
// 成功
{"data": {...}}
// 分页
{"data": [...], "pagination": {"next_cursor": "...", "has_more": true}}
// 错误
{"error": "错误描述"}

## HTTP 约定

- 200 OK、201 Created、400 Bad Request、401 Unauthorized、403 Forbidden、404 Not Found、409 Conflict
- 资产更新需 `If-Match: <version>` 头（乐观锁），版本冲突返回 409
- 游标分页：`?cursor=&limit=50`（limit 默认 50，最大 200）
