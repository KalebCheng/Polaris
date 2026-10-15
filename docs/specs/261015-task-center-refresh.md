# 任务中心（settings › tasks）重构 · Spec

**Created**: 2026-10-15
**Version**: 1.0
**Author**: Agent
**Status**: Completed（2026-10-15 实施完成：后端 `go test ./...` / `go vet ./...` 全绿，前端 `pnpm test`（typecheck + vitest + build）全绿）

> Status lifecycle: `Draft` → `Awaiting Confirmation` → `Confirmed` → `In Progress` → `Completed`。

## 1. 概述

系统设置 › 任务中心原先只做了一件事：**把 `asyncd_tasks` 这张队列表按状态铺成卡片**，旁边再列一遍定时调度项。
于是页面上只有"第 N 次执行"的碎片，回答不了运维真正要问的两类问题：

1. **任务层面**：系统里到底有哪些任务？哪些是事件驱动的（异步任务）、哪些是按周期跑的（定时任务）？各自堆积多少、失败多少？
2. **单次执行层面**：这个任务最近几次执行分别是什么结果、谁在跑、为什么失败、要不要人工重试？

本阶段把任务中心重构为**两个维度**：

- **任务维度**：异步任务（由业务动作入队，无固定周期）与定时任务（由代码声明、按周期入队），各带执行统计；
- **执行记录**：点进任意任务，看该任务的历史执行明细（状态 / 尝试 / 入队与结束时间 / 执行者 / 失败原因），死信可人工重试。

## 2. 背景（后端现状确认）

用户提问"后端是否就是这样设计的"——**结论：数据结构上早就具备两类任务的差异，但缺一层"任务"聚合与"执行记录"入口**。核实到的事实：

| 事实 | 位置 |
|---|---|
| `asyncd_tasks`：一行 = **一次执行**（`pending` / `active` / `completed` / `archived`），带租约、退避、单飞键 | 迁移 `025` |
| `asyncd_schedules`：**定时调度声明**（`job_name` / `task_type` / `kind` / `interval_seconds` / `daily_times` / `window_minutes` / `enabled` / `next_run_at` / `last_status`），启动时由代码幂等 upsert（当前 4 条） | `internal/app/app.go` |
| **任务类型的权威清单在进程内的 `asyncd.Registry`**（6 个：`maintenance.token_cleanup`、`maintenance.record_cleanup`、`syncer.schedule`、`syncer.run`、`provisioning.retry`、`provision.account`），`Registry.Types()` 此前只用于错误文案 | `internal/service/asyncd/registry.go` |
| 调度器只负责**入队**：`Enqueue(task_type, payload, uniqueKey="schedule:"+job_name)`；执行路径唯一（worker → 处理器）。入队时**没有记录作业名这一列**（本阶段补上，见 §5.1） | `internal/service/asyncd/scheduler.go` |
| 管理端点只有 `GET /asyncd/tasks?status=&type=`、`POST /asyncd/tasks/:id/retry`、`GET /asyncd/schedules`、`PUT /asyncd/schedules/:name` | `internal/handler/asyncd_handler.go` |

由此暴露的三个缺口（本次一并清偿）：

1. **没有"任务"这一层**：界面只能显示"在 `asyncd_tasks` 里有记录的类型"。从未执行过的新任务、以及执行记录被清理过的任务会凭空消失——与"任务不存在"在界面上无法区分；异步任务与定时任务也只是被拆成两段 UI，不是同一维度的两种取值。
2. **没有"任务 → 执行记录"的入口**：只有全局任务列表（可按 `type` 过滤），没有"某个任务的历史执行"，定时任务的归属还得靠 `unique_key` 前缀 `schedule:<job>` 才能辨认（**约定即契约**，改一次格式就断链）。
3. `GET /asyncd/schedules` 直接下发实体（含 `payload`、`created_at` 等界面用不到的字段），与 §3.7 DTO 约定不符。

## 3. 决策记录

- **Q1 要不要为"任务"新建实体表？** 不建。任务的**真相有两处且都已经是代码**：定时任务的声明（`RegisterSchedule`）与异步任务的处理器注册（`Registry.Register`）。新建表意味着"代码声明"与"表内容"两份真相，还得处理漂移；目录在**读取时聚合**（调度项 + 注册表 + 统计）即可，零迁移成本、零漂移面。
- **Q2 "执行记录"的归属键用什么？** 定时任务用 `job_name`，异步任务用 `type`。为此**给 `asyncd_tasks` 补 `job_name` 列**（迁移 032，含历史回填 `unique_key LIKE 'schedule:%'`），而不是继续解析 `unique_key` 前缀：归属是稳定契约，不应藏在字符串约定里。
- **Q3 定时任务的记录里要不要包含"同类型但手工触发"的执行？** 不包含。手工触发（如门户点"立即同步"）产生的 `syncer.run` 属于异步任务维度；定时任务的记录就是"调度器为该作业入队的那批"。两者都能看到，但不混在一起——这正是任务维度存在的意义。
- **Q4 任务目录要不要分页 / 传 total？** 不分页。任务集合是**代码里的封闭集合**（注册表 + 调度声明，量级是十位），分页只会制造"翻页才能看全有哪些任务"的假象。
- **Q5 界面能不能新建 / 编辑任务？** 不能，与今天一致：调度声明与任务类型都由代码声明，界面只提供**启停**与**人工重试死信**。理由见 §1「配置即生效」——"配了但不生效"比"没有该配置"更危险；若将来要支持动态任务，必须同时交付执行器接线与权限模型。
- **Q6 摘要条（等待中/执行中/已完成/死信）保留吗？** 保留，但改为取**全队列**状态直方图：它是"平台队列健康度"，与"任务有哪些"正交，且要求覆盖已下线任务类型的残留记录（这部分不在任务目录里）。

## 4. 功能需求

- **FR-1** 任务目录端点 `GET /asyncd/jobs`：返回任务 + 该任务的执行统计（`total` / `by_status` / `last_run_at`）。
- **FR-2** 目录必须包含**从未执行过**的任务（统计为零）。
- **FR-3** 定时任务的执行记录按 `job_name` 归属：`GET /asyncd/tasks?job=…`。
- **FR-4** 异步任务的执行记录按任务类型归属：`GET /asyncd/tasks?type=…`。
- **FR-5** 任务中心分两个维度展示（异步任务 / 定时任务），切换不重挂载筛选态之外的任何东西。
- **FR-6** 任务卡片显示：身份（类型名或作业名）、维度徽标、四个状态计数、最近执行时间；定时任务另有频率、下次执行与启停开关。
- **FR-7** 卡片「执行记录」打开弹窗：按状态筛选 + 分页 + 明细列（状态 / 尝试 / 入队时间 / 结束时间 / 执行者 / 失败原因），死信行内可重试。
- **FR-8** 调度项响应改为 DTO（不下发 `payload` 等实现细节），nil 集合归一化为空集合。
- **FR-9** 新增端点必须接线到调用点（前端 + 冒烟用例），不留"已实现但从未被调用"的公开 API。

## 5. 设计

### 5.1 数据（迁移 032）

```sql
ALTER TABLE asyncd_tasks ADD COLUMN job_name TEXT NOT NULL DEFAULT '';
-- 'schedule:' 长 9 个字符 → 第 10 个字符起即作业名
UPDATE asyncd_tasks SET job_name = substring(unique_key FROM 10)
  WHERE job_name = '' AND unique_key LIKE 'schedule:%';
CREATE INDEX asyncd_tasks_job_idx ON asyncd_tasks (job_name, id DESC);
```

语义：`job_name` 非空 = 该行是调度器为某个作业入队的执行；空 = 业务动作入队的异步执行。

### 5.2 聚合（`asyncd.Service.Jobs`）

```
调度项（asyncd_schedules） ─┐
任务注册表（Registry.Types）─┤→ 目录：[{name, task_type, scheduled, schedule?, stats}]
统计（asyncd_tasks 聚合）   ─┘
```

- 统计一条 SQL 完成：`SELECT job_name, type, status, COUNT(*), MAX(created_at) FROM asyncd_tasks GROUP BY …`，
  由服务层按归属键（`job_name` 非空 → 作业名，否则 → `type`）合并且取最新时间为 `last_run_at`（分组数极小，成本可忽略）。
- 注册表里**已被某个调度项使用**的类型不再出现在异步维度（否则同一份记录被两处统计，界面会自相矛盾）。

### 5.3 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/asyncd/jobs` | 任务目录（两个维度的统一表达，见 FR-1/2） |
| GET | `/asyncd/tasks?job=&type=&status=&page=&size=` | 某任务的执行记录（`counts` 与列表同作用域） |
| GET | `/asyncd/schedules` | 调度项 DTO（不再下发 `payload`） |

### 5.4 前端

```
features/iam/pages/asyncd-page.tsx            # 维度 Tabs + 摘要条 + 卡片网格
features/iam/components/asyncd-job-card.tsx   # 任务卡片（统计 / 频率 / 启停 / 执行记录入口）
features/iam/components/asyncd-runs-dialog.tsx# 执行记录弹窗（状态筛选 + 分页 + 死信重试）
features/iam/lib/asyncd-status.ts             # 状态词汇与**归属查询**的单一来源（asyncdRunsScope）
```

- 卡片页脚是「执行记录」按钮而不是"整卡可点"：卡片内含启停开关，嵌套可点击区域会同时踩到 a11y 与事件冒泡。
- 归属查询集中在 `asyncdRunsScope`（`scheduled ? {job} : {type}`），页面与弹窗不各写一份。
- 弹窗以任务名做 `key`，"换任务"时筛选与页码回到初始态（不会停在越界页码上显示空列表）。

## 6. 实施记录

**后端**
- 迁移 `032_asyncd_task_job_name`（up/down）+ 实体 `AsyncdTask.JobName` + 仓库列清单 / `Enqueue` / 扫描同步（§3.6 手写列清单一致性）。
- `repository.AsyncdTaskQuery`（`Status` / `Type` / `JobName`）替代 `List`/`Counts` 的位置参数；新增 `Stats`。
- `asyncd.WithJobName` + 调度器入队写入（`scheduler.go`）。
- `asyncd.Service.Jobs` 目录聚合；`handler.AsyncdHandler.Jobs`；`asyncdScheduleDTO` / `asyncdStatsDTO` / `asyncdJobDTO`。
- 路由 `GET /api/v1/asyncd/jobs`（平台管理员）+ `router_test` 断言 + 冒烟用例（含 `?job=` 过滤）。
- 测试：`internal/service/asyncd/jobs_test.go`（两个维度拆分、未执行过的任务在列、被调度使用的类型不重复出现、`job_name` 透传、归属过滤透传）、`internal/handler/dto_test.go`（调度项 DTO 不下发 `payload`、nil 集合归一化）。

**前端**
- `asyncdApi.jobs()` / `tasks({job})`；`AsyncdJob` / `AsyncdJobStats` 类型；`AsyncdTask.job_name`。
- 页面重构为两个维度 + 执行记录弹窗；`ResourceTable` 增加 `emptyTitle` / `emptyDescription`（与 `ResourceCards` 同口径）。
- i18n 中英同步（`asyncd.*` 新增 28 条、删除 8 条随本次重构失效的旧词条），`asyncd-page.test.tsx` 重写为 4 个用例。

## 7. 验收标准

- 任务中心默认进入「异步任务」，切到「定时任务」可见调度声明（频率 / 下次执行 / 启停）。
- 从未执行过的任务在目录里可见且统计为零。
- 点「执行记录」按归属过滤（定时任务 `job`、异步任务 `type`），状态筛选回到第 1 页，死信可重试且重试后目录统计刷新。
- `GET /asyncd/schedules` 响应不再含 `payload`。
- 后端 `go test ./...`、`go vet ./...` 与前端 `pnpm test` 全绿；`scripts/smoke-test.sh` 覆盖新端点。

## 8. 范围外（与已知偏差）

- **任务 CRUD 仍不提供**：调度声明与任务类型由代码声明，界面只启停（Q5）。
- **调度器跳过时 `last_status` 仍记 `enqueued`**：命中单飞键（窗口内已有任务）时代码只打日志，`last_status` 不区分 `skipped`；这是本次**未改动**的既有语义，记录在案（改它需要同时定义 `last_status` 的新取值与界面文案）。
- **执行记录没有清理策略（既有问题，本阶段未动）**：仓库里目前没有任何删除 `asyncd_tasks` 的路径（`maintenance.token_cleanup` / `maintenance.record_cleanup` 清的是令牌与审计记录），已完成与死信记录会无限堆积。本阶段只是把它们**按任务**呈现出来，让堆积可见；清理/归档需要先定义留存期与判定口径，属独立议题。
