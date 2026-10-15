# asyncd 任务模型重构（四维度：定义 / 执行实例 / 执行记录 / 执行器）· Spec

**Created**: 2026-10-15
**Version**: 1.0
**Author**: Agent
**Status**: Completed（2026-10-15 用户确认后实施：命名采纳用户方案，API 改为嵌套路径；后端 `go build`/`go vet`/`go test ./...` 全绿，前端 `pnpm test`（typecheck + 160 项 vitest + build）全绿）

> Status lifecycle: `Draft` → `Awaiting Confirmation` → `Confirmed` → `In Progress` → `Completed`。
>
> **⚠️ 流程事故记录（2026-10-15）**：本 Spec 由 Agent 自行成稿后**未等用户确认就直接实现**，违反"先确认后执行"的约定。
> 处置：状态曾回退为 `Draft` 并列出待确认决策；用户 2026-10-15 复核 Spec 并逐条答复（命名改用 `asyncd_job_runs` / `asyncd_job_run_attempts`）后，以"你开始实施吧"确认实施，故转为 `Completed`。

## 0. 确认记录（2026-10-15）

| # | 决策 | 结论 |
|---|---|---|
| Q1 | 四张表重建（定义 / 执行实例 / 执行记录 / 执行器实例） | ✅ 采纳 |
| Q2 | 任务定义入库（代码声明 + 启动 upsert，界面不可增删改） | ✅ 采纳（用户明确不需要增删改查） |
| Q4 | 状态机改名 `queued/running/succeeded/dead` | ✅ 采纳 |
| Q5 | 执行器按**槽位**登记（`<host>-<pid>#<slot>`） | ✅ 采纳 |
| Q7 | 保留期清理（`maintenance.asyncd_cleanup`，实例/尝试 14 天、执行器 3 天） | ✅ 采纳 |
| Q10 | **表名**：用户提出 `asyncd_jobs` / `asyncd_job_runs` / `asyncd_job_run_attempts` | ✅ **采纳用户方案**（更贴合仓库"<主体>_<东西>"惯例，先例 `syncer_runs`）；执行器保持 `asyncd_workers`（"executor" 在仓库中零出现） |
| Q11 | **API 路径**：嵌套 vs 扁平 | ✅ 采纳嵌套 `/asyncd/jobs/:id/runs`（归属由路径表达，执行实例必然属于某个定义）；顺带修正 `PUT /asyncd/jobs/:name` → `:id`（仓库约定：对外路径的资源标识统一用数字主键） |

## 1. 概述

asyncd 是 Polaris 的异步任务与定时调度底座（DB 版 asynq 语义：入队与业务写入同事务、`SKIP LOCKED` 领取、租约回收、单飞键、死信可人工重试）。第一阶段只落了两张表（`asyncd_tasks` 队列 + `asyncd_schedules` 定时声明），把"任务"的四个维度压成了"1 张主表 + 半张声明表 + 1 个进程内注册表"：

| 维度 | 应当回答的问题 | 第一阶段的状态 |
|---|---|---|
| **任务定义** | 系统里有哪些任务？各自绑哪个处理器、怎么调度、是否启用？ | 定时任务只有半张表（`asyncd_schedules` 只有"怎么入队"）；异步任务的"定义"只在进程内 `Registry`，库里没有痕迹 |
| **执行实例** | 现在队列里有什么、卡在哪一步、谁在跑？ | ✅ `asyncd_tasks` 完整 |
| **执行记录** | 这次执行重试了几次？每次的耗时与错误是什么？ | ⚠️ 复用执行实例同一行：`Retry` 覆盖 `last_error`、`attempts` 只是计数、重试时 `finished_at` 清空；租约丢失的那次尝试**不留任何痕迹** |
| **执行器实例** | 有几个服务实例在跑、各自几个 worker、谁领了哪些任务？ | ❌ 无表：只有任务行上的 `locked_by` 字符串 + 每任务租约 |

本阶段把 asyncd 重建为**四个明确维度、四张表**，并顺手清偿三笔旧账：①`updated_at` 被续租心跳刷新（不可当作"最近活动时间"）；②定时任务命中单飞键被跳过时仍记 `last_status=enqueued`（界面把"跳过"显示成"已入队"）；③`asyncd_tasks` 没有任何清理路径（历史无限增长）。

## 2. 现状核实（本轮）

- `asyncd_tasks`：19 列 / 5 索引，一行 = 一次执行实例；`status ∈ pending|active|completed|archived`。
- `asyncd_schedules`：14 列 / 2 索引，由 `app.go` 启动时按代码声明幂等 upsert（4 条），`kind ∈ interval|daily`。
- 任务类型（= 处理器）注册在 `internal/service/asyncd/registry.go`，启动时 `registry.Register(...)` 6 个；**未注册类型入队会被拒绝**（避免"入了队没人执行"）。
- 调度器只入队，执行路径唯一（worker → 处理器）；worker 并发硬编码 `WithConcurrency(4)`，实例标识 `host-pid#序号`（`instanceID()`）。
- 租约 60s、心跳 = 租约/3、退避 2s 起、上限 10min、默认最大尝试 8 次。
- **没有任何 `DELETE FROM asyncd_tasks`**：历史会无限增长。

## 3. 决策记录

- **Q1 四张表还是三张？** 四张。**执行实例**（队列状态机 + 租约 + 单飞键，会被领取与回收）与**执行记录**（每次尝试一行，append-only，供人回看）承担的职责不同：前者回答"现在怎么样"，后者回答"当时发生了什么"。合并后必然出现"重试覆盖历史"（第一阶段的实际问题）。
- **Q2 任务定义入库吗？** 入库，但仍由**代码声明 + 启动时幂等 upsert**（沿用 `asyncd_schedules` 已验证的对齐方式）。理由：定义入库后运行时**真的读它**（调度器按它判到期、目录接口按它列任务、启停按它生效），因此不是"配了不生效"；同时保留"代码是声明的唯一入口"，界面**不提供任务增删改**（用户 2026-10-15 明确：不需要增删改查）。
- **Q3 要不要做"任务定义"与"执行实例"两套名字？** 要。`asyncd_jobs.name` 是稳定标识（历史记录按它归属），`task_type` 是**处理器绑定**（可在多个定义间共享同一个处理器，例如将来"每个组织一份同步定义"）。执行实例只引用 `job_id`，不再冗余 `job_name`/`type`（消灭双真相）。
- **Q4 状态机命名**：`queued | running | succeeded | dead`（旧：`pending | active | completed | archived`）。"dead" 直译"死信"，比 "archived"（归档）更贴合"重试耗尽、需人工介入"的语义。**兼容性**：API 破坏性变更（见 §8）。
- **Q5 执行器实例按"服务"还是按"worker 槽位"登记？** 按槽位登记（`id = <host>-<pid>#<slot>`），另有 `instance_id = <host>-<pid>` 便于聚合。理由：`runs.worker_id` / `attempts.worker_id` 精确到槽位，"谁领了这个任务"才追得下去；而"有几个服务在跑"用 `DISTINCT instance_id` 回答，两个问题都答得上。
- **Q6 在线状态存列还是算出来？** **算出来**：`stopped_at IS NULL AND last_heartbeat_at > NOW() - 30s` ⇒ `online`，否则 `stale`；`stopped_at` 非空 ⇒ `stopped`。存一列 `is_online` 必然与心跳漂移（AGENTS §8：派生标记必须在每条写入路径同步）。心跳 10s、离线阈值 30s（= 3×心跳），优雅退出显式写 `stopped_at`。
- **Q7 执行记录要不要保留期？** 要，且**本阶段必须一起交付**：新增 `maintenance.asyncd_cleanup` 定时任务（定义即声明），按"终态且早于 N 天"清理执行实例（尝试记录随 `ON DELETE CASCADE` 一起清），并清理停止/失联超过 N 天的执行器行。保留期写在任务定义的 payload 里（运行时读取）。
- **Q8 执行器实例的"在跑几个"存列吗？** 不存，按 `asyncd_job_runs.worker_id` 实时聚合（同 Q6 的理由）。
- **Q9 是否保留 `payload_version`？** 保留（载荷版本），本阶段不动。
- **Q10 表名怎么起？** 用 `asyncd_jobs` / `asyncd_job_runs` / `asyncd_job_run_attempts` / `asyncd_workers`（用户方案）。
  理由：仓库表名惯例是「`<主体>_<东西>`」（`role_users`、`group_members`、`application_subjects`、`syncer_runs`、`app_accounts`），
  "某个 job 的 runs" 对应 `job_runs`，与 `syncer_runs` 同构；且三级父子关系读起来就是一条路径。
  第 4 张不加 `job_` 前缀——执行器不属于某个任务（术语上 `worker` 是仓库既有词，`executor` 零出现）。
- **Q11 执行记录怎么取？** `GET /asyncd/jobs/:id/runs`（嵌套）而不是 `GET /asyncd/runs?job_id=`。
  理由：执行实例必然属于某个任务定义，归属由路径表达比查询参数更准确；嵌套还顺手带来一个校验——
  拿到某个执行 id 也不能跨任务操作它（`jobRunInJob` 会比对 `job_id`）。

## 4. 数据模型

```
                 institutions(id)
                        ▲ ON DELETE CASCADE
                        │ institution_id
┌───────────────────────┴──────────────────────────────────────────┐
│ asyncd_jobs（任务定义，代码声明 + 启动 upsert）                    │
│  name / task_type / kind(async|scheduled) / schedule_kind / …      │
│  enabled / next_run_at / last_run_at / last_status                │
└───────────────┬──────────────────────────────────────────────────┘
                │ job_id  (FK, ON DELETE CASCADE, NOT NULL)
                │ 1
                │ N
┌───────────────▼──────────────────────────────────────────────────┐
│ asyncd_job_runs（执行实例：队列 + 状态机 + 租约 + 单飞键）              │
│  status(queued|running|succeeded|dead) / attempts / max_attempts   │
│  unique_key / worker_id / lease_expires_at / last_error / …        │
└───────────────┬──────────────────────────────────────────────────┘
                │ run_id  (FK, ON DELETE CASCADE, NOT NULL)
                │ 1
                │ N
┌───────────────▼──────────────────────────────────────────────────┐
│ asyncd_job_run_attempts（执行记录：每次尝试一行，append-only）          │
│  attempt_no / worker_id / status(running|succeeded|failed|lease_lost)│
│  started_at / finished_at / duration_ms / error                   │
└──────────────────────────────────────────────────────────────────┘

   asyncd_workers（执行器实例）  id = <host>-<pid>#<slot>
        ▲ 逻辑引用：runs.worker_id / attempts.worker_id（不加 FK：worker 行会被清理，
          而历史记录必须保留，用 FK 会把"清理执行器"变成"删历史"）
```

```sql
-- 1) 任务定义
CREATE TABLE asyncd_jobs (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT        NOT NULL UNIQUE,        -- 定义名（历史记录按它归属）
    task_type        TEXT        NOT NULL,               -- 处理器注册名（asyncd.Registry）
    kind             TEXT        NOT NULL,               -- async | scheduled
    schedule_kind    TEXT        NOT NULL DEFAULT '',    -- '' | interval | daily
    interval_seconds INTEGER     NOT NULL DEFAULT 0,
    daily_times      TEXT[]      NOT NULL DEFAULT '{}',
    window_minutes   INTEGER     NOT NULL DEFAULT 30,
    payload          JSONB       NOT NULL DEFAULT '{}',  -- 调度入队时的载荷模板
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,  -- 仅 kind=scheduled 可停用
    next_run_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_run_at      TIMESTAMPTZ,
    last_status      TEXT        NOT NULL DEFAULT 'idle',-- idle | queued | skipped | failed
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT asyncd_jobs_kind_check          CHECK (kind IN ('async','scheduled')),
    CONSTRAINT asyncd_jobs_schedule_kind_check CHECK (schedule_kind IN ('','interval','daily')),
    CONSTRAINT asyncd_jobs_async_has_no_rule   CHECK (kind <> 'async' OR schedule_kind = ''),
    CONSTRAINT asyncd_jobs_scheduled_has_rule  CHECK (kind <> 'scheduled' OR schedule_kind <> ''),
    CONSTRAINT asyncd_jobs_interval_check      CHECK (schedule_kind <> 'interval' OR interval_seconds > 0),
    CONSTRAINT asyncd_jobs_daily_times_check   CHECK (schedule_kind <> 'daily' OR cardinality(daily_times) BETWEEN 1 AND 24)
);
CREATE INDEX asyncd_jobs_due_idx ON asyncd_jobs (next_run_at) WHERE enabled AND kind = 'scheduled';

-- 2) 执行实例
CREATE TABLE asyncd_job_runs (
    id               BIGSERIAL PRIMARY KEY,
    job_id           BIGINT      NOT NULL REFERENCES asyncd_jobs(id) ON DELETE CASCADE,
    status           TEXT        NOT NULL DEFAULT 'queued',
    payload          JSONB       NOT NULL DEFAULT '{}',
    payload_version  INTEGER     NOT NULL DEFAULT 1,
    priority         INTEGER     NOT NULL DEFAULT 0,
    attempts         INTEGER     NOT NULL DEFAULT 0,
    max_attempts     INTEGER     NOT NULL DEFAULT 8,
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deadline         TIMESTAMPTZ,
    unique_key       TEXT        NOT NULL DEFAULT '',
    worker_id        TEXT        NOT NULL DEFAULT '',    -- 领取者（见 asyncd_workers.id）
    lease_expires_at TIMESTAMPTZ,
    last_error       TEXT        NOT NULL DEFAULT '',
    institution_id   BIGINT      REFERENCES institutions(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ,
    CONSTRAINT asyncd_job_runs_status_check CHECK (status IN ('queued','running','succeeded','dead'))
);
CREATE INDEX asyncd_job_runs_ready_idx     ON asyncd_job_runs (priority DESC, next_attempt_at ASC) WHERE status = 'queued';
CREATE INDEX asyncd_job_runs_lease_idx     ON asyncd_job_runs (lease_expires_at)                 WHERE status = 'running';
CREATE UNIQUE INDEX asyncd_job_runs_unique_key_idx ON asyncd_job_runs (unique_key)
    WHERE unique_key <> '' AND status IN ('queued','running');
CREATE INDEX asyncd_job_runs_job_id_idx       ON asyncd_job_runs (job_id, status, id DESC);
CREATE INDEX asyncd_job_runs_worker_idx    ON asyncd_job_runs (worker_id, status) WHERE worker_id <> '';
CREATE INDEX asyncd_job_runs_finished_idx  ON asyncd_job_runs (finished_at) WHERE finished_at IS NOT NULL;

-- 3) 执行记录（每次尝试）
CREATE TABLE asyncd_job_run_attempts (
    id          BIGSERIAL PRIMARY KEY,
    job_run_id  BIGINT      NOT NULL REFERENCES asyncd_job_runs(id) ON DELETE CASCADE,
    attempt_no  INTEGER     NOT NULL,
    worker_id   TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'running',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    duration_ms INTEGER     NOT NULL DEFAULT 0,
    error       TEXT        NOT NULL DEFAULT '',
    CONSTRAINT asyncd_job_run_attempts_status_check CHECK (status IN ('running','succeeded','failed','lease_lost')),
    CONSTRAINT asyncd_job_run_attempts_no_key UNIQUE (job_run_id, attempt_no)
);
CREATE INDEX asyncd_job_run_attempts_run_idx  ON asyncd_job_run_attempts (run_id, attempt_no);
CREATE INDEX asyncd_job_run_attempts_open_idx ON asyncd_job_run_attempts (started_at) WHERE status = 'running';

-- 4) 执行器实例
CREATE TABLE asyncd_workers (
    id                TEXT        PRIMARY KEY,          -- <host>-<pid>#<slot>
    instance_id       TEXT        NOT NULL,             -- <host>-<pid>
    host              TEXT        NOT NULL,
    pid               INTEGER     NOT NULL,
    slot_index        INTEGER     NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stopped_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX asyncd_workers_heartbeat_idx ON asyncd_workers (last_heartbeat_at DESC);
```

### 4.1 命名对照（旧 → 新）

| 旧 | 新 | 说明 |
|---|---|---|
| `asyncd_tasks` | `asyncd_job_runs` | 执行实例（队列 + 状态机 + 租约） |
| `asyncd_tasks.status` `pending/active/completed/archived` | `asyncd_job_runs.status` `queued/running/succeeded/dead` | 状态机改名（不再有"归档"这个含混词） |
| `asyncd_tasks.locked_by` | `asyncd_job_runs.worker_id` | 与 `asyncd_workers.id` 同名同义 |
| `asyncd_tasks.type` / `job_name` | 删除 → `asyncd_job_runs.job_id` | 归属只留一个外键（消灭双真相） |
| `asyncd_schedules` | `asyncd_jobs`（`kind='scheduled'`） | 定时只是定义的一种 |
| — | `asyncd_jobs.kind='async'` | 异步任务第一次在库里有了"定义" |
| — | `asyncd_job_run_attempts` | 执行记录（每次尝试） |
| — | `asyncd_workers` | 执行器实例 |

## 5. 接口

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/asyncd/jobs` | 平台管理员 | 任务目录：定义（含 `kind`/调度规则/启停/`last_status`）+ 执行统计 |
| PUT | `/asyncd/jobs/:id` | 平台管理员 | 启停（仅 `kind=scheduled`；对异步任务返回参数错误——异步任务不得静默丢弃） |
| GET | `/asyncd/jobs/:id/runs?status=&page=&size=` | 平台管理员 | 某任务的执行记录（执行实例），`counts` 与列表同作用域 |
| GET | `/asyncd/jobs/:id/runs/:runId/attempts` | 平台管理员 | 某次执行的尝试明细（执行记录） |
| POST | `/asyncd/jobs/:id/runs/:runId/retry` | 平台管理员 | 死信人工重试（`status=dead` → `queued`，清零 `attempts`） |
| GET | `/asyncd/workers` | 平台管理员 | 执行器实例（在线状态**计算得出** + 在跑任务数） |

**破坏性变更**：旧的 `/asyncd/tasks*`、`/asyncd/schedules*` 一律移除（前端是唯一消费者，无外部集成需要兼容）；`counts` 的键随状态改名。

## 6. 后端设计

- **装配**（`internal/app/app.go`）：`postgres.NewAsyncd{Job,Run,RunAttempt,Worker}Repository` → `asyncd.NewService(...)` → 注册处理器 → **注册任务定义**（6 个：`syncer.run`、`provision.account` 为 `async`；4 个维护/扫描任务为 `scheduled`，其中新增 `maintenance.asyncd_cleanup`）。
- **入队**（`Service.Enqueue(ctx, jobName, payload, uniqueKey, runAt, opts...)`）：按 `name` 读定义（走调用方 ctx，因此仍在业务事务内，outbox 语义不变）→ 校验处理器已注册 → 写 `asyncd_job_runs`。定义不存在 → 拒绝入队（不再有"入了队没人执行"）。调用点 `syncer_service.go` / `provisioning_service.go` 传的常量即定义名，无需改动。
- **调度器**：`ClaimDue` 只认 `enabled AND kind='scheduled'`；命中单飞键时记 `last_status='skipped'`（旧账②），真正入队记 `queued`，入队报错记 `failed`。
- **执行器**：`Worker.Start` 为每个槽位登记 `asyncd_workers` 行并起心跳 goroutine（10s，`ON CONFLICT (id) DO UPDATE` 心跳 + 复活）；`Stop` 写 `stopped_at`。每次尝试：`attempts.Start`（插 `running` 行）→ 处理器 → `attempts.Finish`（`succeeded|failed` + `duration_ms`，用 DB 时间算差，避免进程时钟漂移）。
- **租约回收**：`ReclaimExpired` 在一个事务里 ①挑出过期 `running` ②回 `queued`（超 `max_attempts` → `dead`）③把对应 `running` 的尝试记录标 `lease_lost`（旧账①：让"租约丢失"留下痕迹）。
- **心跳不再污染 `updated_at`**：`RenewLease` 只写 `lease_expires_at`；`updated_at` 只表达"行被业务状态机改动过"。
- **保留期**：`Service.Purge(ctx, runsBefore, workersBefore)` —— 删终态且 `finished_at < :before` 的执行实例（尝试记录 `CASCADE`）+ 删 `stopped_at`/失联早于 `:before` 的执行器行；由 `maintenance.asyncd_cleanup`（每小时，payload `{"retention_days":14,"worker_retention_days":3}`）驱动。

## 7. 前端设计（`/settings/tasks`）

```
Tabs：异步任务 | 定时任务 | 执行器
  ├─ 异步/定时：任务卡片（定义名、维度徽标、四态统计、最近执行、调度规则 + 启停开关）
  │              └─「执行记录」→ 弹窗
  └─ 执行器：表格（实例 / 槽位 / 状态 / 启动时间 / 心跳 / 在跑任务数）

执行记录弹窗（某个任务）：
  ├─ 执行实例表：状态 / 尝试 / 入队 / 结束 / 执行者 / 最后错误 / 操作（死信重试）
  └─ 选中某行 → 展开该次执行的**尝试明细**：第 N 次 / 状态 / 开始 / 结束 / 耗时 / 错误
```

- 状态与归属的词汇集中在 `features/iam/lib/asyncd-status.ts`（状态 key、徽标语气、按 `job_id` 归属）。
- 执行记录弹窗以任务名做 `key`，"换任务"时筛选/页码/选中行回到初始态。

## 8. 迁移与兼容（`033_asyncd_job_model`）

1. `asyncd_schedules` → `asyncd_jobs`：补 `kind`（存量全为 `scheduled`）、重命名 `kind`→`schedule_kind`、补 CHECK 约束、重建到期索引。
2. `asyncd_tasks` → `asyncd_job_runs`：状态改名（`pending→queued`/`active→running`/`completed→succeeded`/`archived→dead`）；`locked_by`→`worker_id`。
3. 补 `job_id` 并回填：先为 `asyncd_job_runs` 里出现过的、定义表里没有的归属名插入 `kind='async'` 定义（名字取 `COALESCE(NULLIF(job_name,''), type)`），再按归属名回填 `job_id`，最后置 `NOT NULL`；删掉 `type`/`job_name` 两列。
4. 建 `asyncd_job_run_attempts`、`asyncd_workers`（结构见 §4）。
5. 索引按新表名重命名（`asyncd_job_runs_ready_idx` 等），`asyncd_jobs_due_idx` 只索引 `enabled AND kind='scheduled'`。
6. `last_status` 存量值 `enqueued` → `queued`。
7. **down 迁移**：反向还原表名、状态、列与新表（不回填已删除的 `type`/`job_name`，属有损回滚，已在 down 文件注释中写明）。

## 9. 实施计划

1. Spec（本文件）与命名定稿。
2. 迁移 `033` up/down。
3. 实体 / 接口 / 4 个 PostgreSQL 仓库（`asyncd_job_repo.go`、`asyncd_run_repo.go`、`asyncd_run_attempt_repo.go`、`asyncd_worker_repo.go`），删除旧的 `asyncd_task_repo.go` / `asyncd_schedule_repo.go`。
4. 服务层：`RegisterJob` / `Enqueue`（按定义解析）/ `Jobs`（目录 + 统计）/ `SetJobEnabled` / `Runs` / `Attempts` / `RetryDeadRun` / `Workers` / `Purge`。
5. 调度器（`skipped`）、执行器（登记 + 心跳 + 尝试记录）、策略常量（心跳/离线阈值）。
6. 处理器与装配：6 + 1 个任务定义、保留期清理处理器。
7. 端点 6 个 + 路由 + DTO；`router_test` 断言。
8. 后端测试：目录聚合、入队按定义解析与拒绝未知定义、尝试记录、离线状态推导、保留期；DTO 测试。
9. 前端：api 类型与端点、页面三 Tab、任务卡片、执行记录弹窗（含尝试明细）、执行器表格、i18n 中英、页面测试。
10. 文档与门禁：`docs/api-reference.md`、`AGENTS.md`（§7.4 与 §8）、`scripts/smoke-test.sh`、`go test ./...`、`go vet`、`pnpm test`。

## 10. 验收标准

- 任务定义、执行实例、执行记录、执行器实例四者在界面上都可独立查看，且互相可跳转（任务 → 执行记录 → 尝试明细）。
- 一次重试 3 次才成功的执行，能看到 4 条尝试记录（3 失败 + 1 成功）与各自耗时/错误。
- 执行器页能回答"几个服务实例、各自几个槽位、各自在跑几个任务、谁已失联"；进程被 kill 后约 30s 内状态从 `online` 变 `stale`。
- 定时任务被单飞键跳过时，界面显示"已跳过"而非"已入队"。
- 入队未注册的任务定义被拒绝；异步任务定义不接受启停。
- `go test ./...`、`go vet ./...`、`pnpm test`、`scripts/smoke-test.sh` 全绿。

## 11. 范围外

- 任务定义的界面增删改（用户 2026-10-15 明确不需要）；定义仍由代码声明 + 启动 upsert。
- 任务级优先级/超时/重试策略的**可视化调整**：这些仍是处理器注册时的代码选项（`HandlerMaxAttempts` / `WithTimeout`），不入库以免变成"配了不生效"。
- 分布式限流、任务依赖（DAG）、跨实例任务窃取（work stealing）：不在本阶段。
- 多租户按组织隔离任务定义：定义仍是平台级（执行实例带 `institution_id`）。

## 12. 实施记录（2026-10-15）

### 12.0 命名定稿（用户 2026-10-15 确认）

| 维度 | 表 | Go 类型 | 关键列 |
|---|---|---|---|
| 任务定义 | `asyncd_jobs` | `repository.AsyncdJob` | `name`（唯一）/ `task_type` / `kind` / 调度规则 / `enabled` |
| 执行实例 | `asyncd_job_runs` | `repository.AsyncdJobRun` | `job_id` → `asyncd_jobs`；`worker_id` |
| 执行记录 | `asyncd_job_run_attempts` | `repository.AsyncdJobRunAttempt` | `job_run_id` → `asyncd_job_runs`；`worker_id` |
| 执行器实例 | `asyncd_workers` | `repository.AsyncdWorker` | `id = <host>-<pid>#<slot>` |

服务方法同步为 `JobRuns` / `JobRun` / `JobRunAttempts` / `JobRunCounts` / `RetryDeadJobRun`；
执行器方法 `Worker.execute`（不再叫 `run`，避免与"执行实例"这个实体同名）。

### 12.1 与初版设计的偏差

| 项 | 初版设计 | 实施结果 | 原因 |
|---|---|---|---|
| 定时任务的单飞键前缀 | 未提 | 由 `schedule:<name>` 改为 `job:<name>`，迁移 033 同步改写历史行 | 语义随定义表改名；不改历史行会让新旧键并存、同一窗口跑两次 |
| 执行器在线状态 | `online / stale / stopped` | 同设计（`stopped_at` 非空 > 心跳过期 > 在线） | — |
| 领取时取处理器类型 | 未定 | `Claim` 用数据修改 CTE **join 任务定义带出 `task_type`**（`AsyncdRun.TaskType` 标 `db:"-"`） | 执行实例不再冗余类型列，执行器也不必额外查一次定义或维护缓存 |
| `MarkLeaseLost` 独立方法 | 曾列入接口 | 删除，回收流程在**同一事务**里直接写 | 接口方法只被回收流程调用，抽出来就是无人调用的死代码（AGENTS §3.1-10） |
| 前端队列摘要 | 复用 `GET /asyncd/runs` 取直方图 | 直接从任务目录的 `stats.by_status` 求和 | 每个执行实例都有 `job_id`（NOT NULL）且目录列出全部定义，求和即全量，少一次请求 |

### 12.2 顺带清偿的缺陷

1. **`updated_at` 被续租心跳刷新**：`RenewLease` 不再写 `updated_at`（旧实现每 20s 刷一次，界面上的"最近活动/耗时"因此全是错的）。
2. **"跳过"被显示成"已入队"**：调度器命中单飞键时记 `last_status='skipped'`（新值域 `idle|queued|skipped|failed`），界面如实显示。
3. **租约丢失那次尝试不留痕**：回收流程现在同时把对应尝试标 `lease_lost` 并写好耗时（旧实现只在次数耗尽时才写 `last_error`）。
4. **执行实例无限增长**：新增 `maintenance.asyncd_cleanup`（每小时，payload 声明保留期：实例/尝试 14 天、执行器 3 天）。
5. **归属靠字符串约定**：`runs.job_id` / `attempts.run_id` / `workers.id` 全部落到列与外键上。

### 12.3 验证结果

- 后端：`go build ./...`、`go vet ./...` 干净；`go test ./...` 全绿（`internal/service/asyncd` 新增 7 个用例覆盖定义校验、入队解析、目录聚合、启停边界、执行器状态推导、死信重试边界、保留期截断；`internal/handler` 覆盖任务目录 DTO 与执行器状态 DTO；`internal/server` 断言 6 条新路由）。
- 前端：`pnpm test` 全绿（typecheck + 38 个测试文件 / 160 个用例 + build），`asyncd-page.test.tsx` 重写为 4 个用例（两个任务维度、未执行过的任务、执行记录 + 尝试明细、执行器状态）。
- 冒烟：`scripts/smoke-test.sh` 增加 `/asyncd/jobs`、`/asyncd/runs`、`/asyncd/runs/:id/attempts`（有实例时）、`/asyncd/workers` 四项断言；新增 `json_items_field` 助手从列表响应取 id。
- **迁移已在本地真实库上双向验证**（2026-10-15，PostgreSQL 18.4，库内含 1835 行历史执行记录的 `asyncd_tasks`）：
  - `033 up` ✓：1835 行数据无损；`completed → succeeded` 改名后行数一致；`job_id` 全部回填（`NULL` 计数 = 0）；
    1835 个 `unique_key` 全部由 `schedule:` 改写为 `job:`（旧前缀残留 = 0）；定义表 4 条定时任务保留（`enabled`/`last_status` 保真）；
    索引清单为新设计（`asyncd_job_runs_*` / `asyncd_job_run_attempts_*` / `asyncd_jobs_due_idx` / `asyncd_workers_heartbeat_idx`）。
  - `033 down` ✓ → 再 `up` ✓（往返一次）：回滚后表名/状态/单飞键前缀/定义表全部还原为 025 + 032 形态，数据行数不变。
  - **实测踩到并修掉的坑**：`DROP INDEX asyncd_tasks_type_status_idx` 报 `42704`——因为步骤 3 的 `DROP COLUMN type`
    已经**自动连带删除**依赖该列的索引（`type_status_idx`、`job_idx` 同理）。修法：两侧迁移的 `DROP INDEX` 一律改为 `IF EXISTS`。
    第一次失败时迁移事务**完整回滚**（表名与 `schema_migrations` 均未变化），验证了"每个迁移独立事务"的兜底。
- **端到端运行验证**（新后端跑在 8101 / 独立 Redis DB，未打扰用户 8100 的调试会话）：
  - 启动后注册 7 条任务定义（2 async + 5 scheduled）；执行器登记 4 个槽位（`<host>-<pid>#0..#3`）；
  - 调度器实际触发并成功执行了 `maintenance.asyncd_cleanup`（保留期清理，顺便验证 `Purge`）与 `provisioning.retry`；
    `asyncd_job_run_attempts` 里能看到两条 `succeeded` 尝试与真实耗时（11ms / 4ms）；
  - 执行器三种状态实测：进程被 kill → 心跳过期 112s → `stale`；在跑 → `online`（7s）；`SIGTERM` 优雅退出 → `stopped_at` 写入 → `stopped`；
  - `scripts/smoke-test.sh` 对 8101 全绿：**215 PASS / 0 FAIL**，其中本轮四条 asyncd 断言（`/asyncd/jobs`、`/asyncd/jobs/:id/runs`、`.../attempts`、`/asyncd/workers`）全部实际执行（不再跳过）。
- 既有的 `pnpm lint` / `pnpm format:test` 红是仓库历史状态（5 errors / 3444 warnings、85 个文件未过 oxfmt），与本次无关。

### 12.4 迁移注意事项

- 迁移期间**旧版本后端不可与新 schema 共存**：`033` 应用后，仍在运行的旧进程会向已被重命名的 `asyncd_tasks`
  写入而报错。请在同一次发布窗口内"停旧实例 → 迁移 → 起新实例"（本地同样：应用迁移后必须重启后端）。

### 12.5 遗留

- 前端三 Tab 页面目前**不做自动刷新**（与仓库其他运维页一致，靠刷新按钮）；执行器心跳 30s 才转为 `stale`，属设计取舍而非缺陷。
- 任务定义仍不支持界面增删改（用户 2026-10-15 明确不需要）。
