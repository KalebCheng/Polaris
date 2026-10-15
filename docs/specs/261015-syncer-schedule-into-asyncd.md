# 身份连接的「同步计划」收敛进 asyncd 定时任务 · Spec（第七阶段）

**Created**: 2026-10-15
**Version**: 0.1（待确认）
**Author**: Agent
**Status**: Completed（2026-10-15 用户确认"你开工吧"后实施：迁移 034、asyncd cron + 归属任务、连接计划收敛、前端来源与 cron 展示；后端 `go test`/`go vet` 全绿、前端 `pnpm test` 全绿、真实库 up/down 双向验证 + API 端到端 15 项断言通过）

> Status lifecycle: `Draft` → `Awaiting Confirmation` → `Confirmed` → `In Progress` → `Completed`。
> 用户 2026-10-15 选定路线 **C**（把 syncer 的定时校验收敛成 asyncd 的定时任务）并给出机制草案：
> "保存时后端去 asyncd 创建一个定时任务并关联它的 id，修改时也改这个任务"。
> 本 Spec 采纳该方向，但**修正了机制上的三处坑**（§0 的 Q1/Q5/Q7、§3.2），并在实施后按实际落地情况回填 §8。

## 0. 决策记录（用户 2026-10-15 以"你开工吧"确认采用下列建议）

> 下表是确认时的结论；实施中的三处调整见 §8.1。

| # | 决策 | 我的建议 | 备选 |
|---|---|---|---|
| Q1 | **关联方向**：job 上存 `owner_syncer_id`（推荐）还是 syncer 上存 `schedule_job_id` | job 侧存 owner：单一真相 + `UNIQUE(owner_syncer_id)` 天然保证"一个连接一个计划" + `ON DELETE CASCADE` 兜住孤儿任务 | syncer 侧存 job_id（你若更想要，需要额外的唯一约束与一致性维护） |
| Q2 | **计划改回「不自动」/ 连接停用 / 连接删除** 时任务怎么办 | 「不自动」与「停用」→ `enabled=false`（保留定义与历史）；删除连接 → FK 级联删定义 | 「不自动」→ 直接删定义（**会连历史执行记录一起删**，见 §3.3 反例） |
| Q3 | **代码声明的定义退役**（本次要下线的 `syncer.schedule` 扫描任务） | 加 `retired_at`：启动时把"未被本次声明且无 owner"的定义标退役，历史保留、目录默认不列 | 直接删定义（会 CASCADE 掉它的执行历史，本地库已有 1191 条） |
| Q4 | **一跳还是二跳** | **一跳**：连接的计划任务 `task_type` 直接指向既有的 `syncer.run` 处理器，"到点即同步" | 二段跳（调度任务再派发 `syncer.run`）——一次同步在任务中心会出现两条记录 |
| Q5 | **单飞键**：定时与手工触发是否共用一个键 | 定义上新增 `unique_key` 列，连接的计划任务写 `syncer:<id>`（与手工触发同一个键）→ 两者不会并发跑两次 | 沿用 asyncd 默认的 `job:<name>`（挡不住"手工 + 定时"同时跑） |
| Q6 | **改计划何时生效** | 规则变化时重置 `next_run_at=NOW()`，验收"改完 ≤1 分钟生效" | 不动 `next_run_at`（最长要等到旧的下次时间，等于"配了不生效"） |
| Q7 | **权限口径** | 承认并写明：`syncer:update`（组织级）从此能间接改动平台级 asyncd 目录里的条目；任务中心仍是平台管理员"只读 + 启停" | 计划改动收敛到平台管理员（与今天"连接自己配计划"的体验冲突） |
| Q8 | **Cron 能力边界** | 沿用现有 `internal/service/cron`：5 段（分 时 日 月 周）、支持 `*`/数值/列表/区间/步长；不支持秒、`JAN`/`MON`、`@daily`、`L/W/#`。保存时拒绝非法表达式；迁移进来的非法值 → 调度器跳过并记 `last_status=failed`（目录可见） | 引入 robfig/cron 支持 6 段与别名（新增依赖） |

## 1. 概述

今天仓库里有**两套"定时"语义**，是用户困惑的根源，也是本阶段要消灭的东西：

| 层 | 规则 | 判定者 | 代码 |
|---|---|---|---|
| asyncd 任务定义 | `interval` / `daily`（+ 执行窗口） | 调度器（`ClaimDue` + `SKIP LOCKED`，多实例安全） | `internal/service/asyncd/{policy,scheduler}.go` |
| 身份连接的「定时校验」 | `interval` / `daily` / **`cron`** | 每分钟运行的扫描任务 + Go 侧 `DueForSync` | `internal/service/syncer_schedule.go`、`internal/app/jobs.go` |

本阶段把连接的计划**变成 asyncd 的一条定时任务定义**（一个连接一条），于是：
**Cron 自然成为 asyncd 能力的一部分**（任务中心的定时任务卡片直接显示表达式），
`syncer.schedule` 扫描任务与 `DueForSync` **整体退役**（消灭两套语义）。

## 2. 现状核实

- 连接的计划存在 `syncers.sync_config.schedule`（JSONB）：`{mode:''|'interval'|'daily'|'cron', interval_seconds, times[], cron, window_minutes}`（`internal/repository/models.go:669`）；
  保存时由 `validateSyncSchedule` 校验（`internal/service/syncer_service.go:291`，cron 走 `cron.Parse`）。
- 运行时：`syncer.schedule`（每 60s，代码声明）→ `runScheduledSyncers` 列出启用连接 → `DueForSync` 判"该不该跑" → 入队 `syncer.run`（单飞键 `syncer:<id>`，`trigger=schedule`）。
- 手工触发 `POST /syncers/:id/run`（`syncer:update`）入队同一个 `syncer.run`，单飞键同样是 `syncer:<id>`。
- 兼容路径：`mode=''` 时退回历史字段 `syncers.schedule`（间隔秒数）。
- 现状规模：本地库 `syncers` 为 0 行（无数据压力），但**生产可能有 N 条 → 迁移必须回填**（见 §4 风险 R1）。
- asyncd 侧：定义表 `asyncd_jobs`（`schedule_kind` CHECK 只允许 `''|interval|daily`）、`RegisterJob` 由代码声明、`NextOccurrence` 只有 interval/daily 两个分支；单飞键由调度器硬编码为 `job:<name>`。

## 3. 设计

### 3.1 数据模型（迁移 034）

```sql
ALTER TABLE asyncd_jobs
  ADD COLUMN cron_expr        TEXT        NOT NULL DEFAULT '',   -- schedule_kind='cron' 时的 5 段表达式
  ADD COLUMN unique_key       TEXT        NOT NULL DEFAULT '',   -- 声明式单飞键（空 = 回落 job:<name>）
  ADD COLUMN owner_syncer_id  BIGINT      REFERENCES syncers(id) ON DELETE CASCADE,
  ADD COLUMN retired_at       TIMESTAMPTZ;                       -- 代码声明退役（保留历史）

-- schedule_kind 放开到 4 种，并按类型要求必填字段
ALTER TABLE asyncd_jobs DROP CONSTRAINT asyncd_jobs_schedule_kind_check;
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_schedule_kind_check
  CHECK (schedule_kind IN ('','interval','daily','cron'));
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_cron_check
  CHECK (schedule_kind <> 'cron' OR cron_expr <> '');
ALTER TABLE asyncd_jobs ADD CONSTRAINT asyncd_jobs_owner_scheduled_check
  CHECK (owner_syncer_id IS NULL OR kind = 'scheduled');

-- 一个连接最多一个计划任务
CREATE UNIQUE INDEX asyncd_jobs_owner_syncer_key ON asyncd_jobs (owner_syncer_id)
  WHERE owner_syncer_id IS NOT NULL;
CREATE INDEX asyncd_jobs_owner_syncer_idx ON asyncd_jobs (owner_syncer_id) WHERE retired_at IS NULL;
-- 到点扫描只认活跃定义
DROP INDEX asyncd_jobs_due_idx;
CREATE INDEX asyncd_jobs_due_idx ON asyncd_jobs (next_run_at)
  WHERE enabled AND kind = 'scheduled' AND retired_at IS NULL;
```

### 3.2 连接保存 → 同步计划任务（**同一事务**）

`SyncerService.Create/Update` 在写 `syncers` 的**同一事务**里（`dbclient.WithTx`，AGENTS §3.1-6）调用
`asyncd.Service.UpsertOwnedJob(...)`，生成/更新一条定义：

| 字段 | 值 |
|---|---|
| `name` | `syncer.schedule:<syncer_id>`（稳定、可重算，不依赖连接改名） |
| `task_type` | `syncer.run`（**复用既有处理器**，注册表按 task_type 索引 → 多个定义可共用一个处理器） |
| `kind` | `scheduled` |
| `schedule_kind` / `interval_seconds` / `daily_times` / `window_minutes` / `cron_expr` | 来自 `sync_config.schedule`（`mode=''` 回退历史 `syncers.schedule` 秒数 → interval） |
| `payload` | `{"syncer_id":N,"institution_id":M,"trigger":"schedule"}` |
| `unique_key` | `syncer:<syncer_id>`（**与手工触发共用**） |
| `enabled` | `连接 is_enabled && 计划非空` |
| `owner_syncer_id` | 连接主键 |

- **规则变化即重置 `next_run_at = NOW()`**（Q6）：否则改了计划要等到旧的下次时间才生效。
- 计划为空（`mode=''` 且无历史间隔）→ 定义 `enabled=false`（Q2：不删，历史要留）。
- 连接删除 → 外键 `ON DELETE CASCADE` 删定义（主体没了，历史随之删是合理的）。

### 3.3 执行链（一跳）

```
asyncd 调度器（每分钟? 不，1s 扫描 ClaimDue）
  → 抢占到点的连接计划任务 → Enqueue(jobName, payload, unique_key='syncer:<id>')
      ↳ 撞单飞键（该连接正在跑 / 已有待执行）→ last_status='skipped'（如实记录）
  → worker 领取 → task_type='syncer.run' 的处理器 → RunWithTrigger('asyncd','schedule',syncer)
  → 写 syncer_runs（历史与手工触发统一）+ asyncd_job_runs/attempts（这次执行的耗时与错误）
```

于是"任务中心的执行记录"就是**同步本身**（不再出现"派发了一次、执行了一次"两条记录）。

**为什么必须共用单飞键（Q5 的坑）**：若沿用 asyncd 默认的 `job:<name>`，它只保证"同一任务窗口内不重复入队"，
**挡不住"运营点立即同步"与"定时到点"同时跑**——今天这条保护正是 `syncer:<id>`。

### 3.4 代码声明 vs 数据声明的边界

- `asyncd_jobs` 里从此有两类行：**代码声明**（`owner_syncer_id IS NULL`，启动 upsert）与**数据声明**（连接持有的计划）。
- 新增 `RetireUndeclared(activeNames)`：启动时把"本次未声明且无 owner"的定义标 `retired_at`（Q3）。
  本次要下线的 `syncer.schedule` 即由此退役（保留它的 1191 条历史执行记录）。
- 目录 `GET /asyncd/jobs` 默认只列 `retired_at IS NULL`；退役项只在历史里可见。

### 3.5 前端

- 任务中心 › 定时任务：卡片新增**来源徽标**（「平台」/「身份连接 · <连接名>」）与来源过滤；`schedule_kind='cron'` 时显示表达式（`asyncd.scheduleCron`）。
- 连接编辑页「同步范围与计划」**字段不变**（仍写 `sync_config.schedule`），保存即同步任务；可选增强：连接详情显示"下次执行"（读任务的 `next_run_at`）。

## 4. 迁移 034 与风险

**up**：加列/约束/索引 → **存量回填**（把每个有计划的连接翻译成一条定义，含 `mode=''` + 历史秒数的兼容路径）
→ 删除代码声明 `syncer.schedule`（由 §3.4 的退役机制处理，迁移本身不删行）。

**down**：删掉回填出的 owner 定义、恢复旧 CHECK 与索引、删新列；代码侧需 git 回滚（`DueForSync` 已删除，schema 回滚不会自动恢复行为）。

| 风险 | 说明 | 对策 |
|---|---|---|
| **R1 升级后存量连接的定时同步静默停摆** | 定义由"连接保存时生成"，若不回填，所有连接在第一次保存前都没有计划任务 | 迁移必须回填（SQL 直接读 `syncers.sync_config` 生成），并在冒烟里断言"迁移后 `/asyncd/jobs` 出现每个连接的计划任务" |
| R2 目录规模 | N 个连接 = N 条定义（本地 0，生产未知） | 前端来源分组/过滤；目录不分页的理由（封闭集合）需要重新评估 |
| R3 非法存量 cron | SQL 无法校验 cron 语法 | 保存时拒绝新值；存量非法 → 调度器跳过 + `last_status='failed'`（目录可见） |
| R4 权限语义变化 | 见 Q7 | 写进 Spec 与 AGENTS §7.4 |
| R5 回滚 | 代码侧删了 `DueForSync`，down 迁移只恢复 schema | 发布窗口内保留上一版二进制；文档写明回滚需同时回滚代码 |

## 5. 实施计划

1. Spec 定稿（本文件）→ 用户确认 Q1–Q8。
2. 迁移 034（up/down）+ 本地/真实库双向验证（含存量回填断言）。
3. asyncd：`Schedule{Kind:"cron", Cron}`、`NextOccurrence` cron 分支（复用 `cron.Parse/Next`）、`Job` 增加 `Cron/UniqueKey/OwnerSyncerID`、`UpsertOwnedJob`、`RetireUndeclared`、调度器用定义的 `unique_key`。
4. syncer：`Create/Update/Delete` 同事务同步定义；删除 `runScheduledSyncers` / `DueForSync` / `syncer.schedule` 声明与处理器（不留死代码）。
5. 前端：来源徽标 + 过滤 + cron 展示 + i18n 中英；任务中心测试补齐。
6. 文档与门禁：`docs/api-reference.md`、`AGENTS.md`（§7.4 任务中心、§8 教训）、冒烟用例（连接保存 → 目录出现来源任务）、`go test` / `go vet` / `pnpm test`。

## 6. 验收标准

- 连接保存后，`GET /asyncd/jobs` 出现该连接的计划任务（来源=身份连接，规则与界面一致）；改计划后 ≤1 分钟生效。
- Cron 表达式（5 段）在任务中心可见、按表达式到点触发；非法表达式保存即被拒。
- 「不自动」/连接停用 → 计划任务 `enabled=false`，历史执行记录保留；删除连接 → 任务与历史一并消失（无孤儿）。
- 手工「立即同步」与定时到点重叠时只跑一次，任务中心如实显示 `skipped`。
- `syncer.schedule` 退役：不再有每分钟扫描；其历史执行记录仍在（`retired_at`）。
- 后端 `go test ./...` / `go vet ./...`、前端 `pnpm test`、冒烟全绿；迁移在真实库 up/down 验证通过。

## 7. 范围外

- **一个连接多个计划**（如"工作日 cron + 周末 daily"）：今天 `sync_config.schedule` 只有一个 mode，`UNIQUE(owner_syncer_id)` 也按一条算；需要时去掉唯一索引即可。
- 组织级"默认同步计划"模板（新建连接自动继承）。
- 把连接的其它配置（同步范围/字段映射）也纳入任务定义——只有**计划**属于调度维度。

## 8. 实施记录（2026-10-15）

### 8.1 与初版设计的偏差

| 项 | 初版设计 | 实施结果 | 原因 |
|---|---|---|---|
| 连接保存与任务定义写入 | §3.2 写"**同一事务**" | 改为**即时写入 + 每分钟对账自愈**（`syncer.schedule.reconcile`） | 本仓库约定"一个仓库方法 = 一个事务"，且 §3.1-2 禁止服务直接用 `dbclient`；跨域事务需要新基础设施（ctx 携带 tx）或让 syncer 仓库写 asyncd 表（跨域 SQL），两者都比"即时写入 + 自愈"更重。保存失败会**如实报错**（不是静默成功），且 1 分钟内被对账纠正 |
| 兼容路径 | 未细化 | 运行时**保留** `syncers.schedule`（历史间隔秒数）回退 | 核对前端后发现该字段仍是「同步范围与计划」里「同步周期」下拉的落库位置（不是死数据）：`mode=''` + 秒数 = 真实计划；两者都空才等于「不自动」 |
| 执行器登记与计划任务的关系 | 未提 | 无 | 执行器与计划任务正交，实施未受影响 |
| 任务定义执行器接口 | 未提 | 新增 `asyncdScheduler` 小接口（Enqueue / UpsertOwnedJob / SetOwnedJobEnabled / OwnedJobs） | 连接服务只用到这四件事；抽出来让对账逻辑可单测（`TestReconcileScheduleJobs`） |

### 8.2 落地清单

- 迁移 `034_asyncd_owner_jobs_and_cron`（up/down）：`asyncd_jobs` 增加 `cron_expr` / `unique_key` / `owner_syncer_id`（→ `syncers(id)` CASCADE）/ `retired_at`；
  `schedule_kind` 放开到 `interval|daily|cron`；一个连接一个计划（部分唯一索引）；到点索引排除退役定义；**存量回填**（cron/daily/interval/legacy 四种形态，按 id 抖动 0..59 秒避免同时到点）；`syncer.schedule` 标记退役。
- asyncd：`NextOccurrence` 支持 cron（非法表达式返回 error → 调度器记 `failed` + 推迟 1 小时）；`UpsertOwnedJob`（规则/启用态变化时重置 `next_run_at`）；`RetireUndeclared`；`SetEnabledByOwner`；调度器单飞键取自定义（`unique_key`，空则回落 `job:<name>`）。
- 连接侧：`syncerScheduleJob` 翻译（mode → 规则，mode='' 回退历史秒数）；`applyScheduleJob`（保存即写入）；`ReconcileScheduleJobs`（对账自愈）；
  删除 `DueForSync` / `runScheduledSyncers` / `syncer.schedule` 声明与处理器（老的"每分钟扫描 + Go 侧判定"整体退役）。
- 前端：任务卡显示 cron 表达式与来源徽标；「定时任务」维度新增来源过滤（全部 / 平台 / 身份连接）；归属连接的任务**不渲染启停开关**；i18n 中英各 +6 条。
- 冒烟：新增步骤 **9a**——创建带 cron 计划的连接 → 目录出现该来源任务且规则/处理器/来源正确 → 归属任务拒绝界面启停（400）→ 删除连接 → 无孤儿任务。

### 8.3 验证结果（真实库 + 真后端）

- 迁移双向：`034 up` 后 5 条覆盖各形态的测试连接生成 4 条定义（cron/daily/interval/legacy 各按其规则，无计划的不生成），`owner_syncer_id`/`unique_key`/payload 全对，`syncer.schedule` 退役；
  `034 down` → 回到 033 形态（新列/新索引消失、`syncer.schedule` 复活），再 `up` 又是同样的 4 条 —— 幂等可重放。
- 调度链：迁移回填的定义在抖动窗口内各跑了一次（`job_runs` 有记录、`attempts` 有真实耗时）；`daily` 的下次时间自动落到次日 03:00，`interval 900` → +15 分钟，legacy `1800` → +30 分钟；
  cron 任务按分钟触发；同一连接的第二个实例被单飞键挡住并记 `skipped`（失败原因如实落库：测试连接指向不可达 LDAP → 8 次尝试后进死信）。
- API 端到端 15 项断言全过：登录 → 创建连接（cron）→ 目录出现 `syncer.schedule:<id>`（来源名可读、`task_type=syncer.run`）→ 归属任务启停被拒（400）→ 计划改「不自动」→ 任务保留但停用 → 改回 cron → 重新启用 → 删除连接 → 无孤儿任务。
- **混合版本部署的真实观测**（已写进 AGENTS §8）：旧实例会把 `syncer.schedule.reconcile` 记成"没有处理器"的死信，并按 interval 语义把 cron 任务的 `next_run_at` 推进成 60 秒 —— 发布必须"停旧实例 → 迁移 → 起新实例"。

### 8.4 遗留

- 对账任务每 60 秒扫全部连接（`List` 上限 500）+ 全部归属定义做 diff；连接数量级上千时再考虑增量（按 `updated_at` 水位）。
