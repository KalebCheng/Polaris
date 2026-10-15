# 控制台前端 UI 重构专题 Spec（框架 → 基础组件 → 功能页面）

**Created**: 2026-10-15
**Version**: 1.0
**Author**: Agent（用户提供参考视觉 + 三步走范围）
**Status**: Completed

## 1. 概述

按用户给出的两屏参考视觉（招聘 / 员工管理工作台），把 Polaris 控制台前端整体换一次皮，
并**分三步**推进：**① 框架 UI（外壳与骨架）→ ② 基础组件 UI → ③ 功能页面 UI**。

目标不是"多几个圆角"，而是建立一套可延续的视觉体系：**画布 / 面板 / 反相强调**三层材质，
固定档位的圆角与投影，主色只留给动作；此后新页面直接组合组件即可得到一致观感，
不再需要逐页调间距与色值。

**明确不改**：信息架构（平台级 + 组织工作区两层导航）、路由、权限口径、i18n 结构与后端契约。
这一轮是**纯前端表现层重构**，除修复"总览页身份连接入口指向不存在路由"一处死链外，无行为变更。

## 2. 背景

### 2.1 用户诉求（原话）

> 我们来一个前端 UI 重构专题，请按照这个视觉效果按照以下步骤进行：1. 框架UI实现 2. 基础组件 UI重构实现 3. 功能页面 UI 重构实现

附加两张参考截图（工作台首页 / 招聘与录用列表页）。

### 2.2 参考视觉的客观提取（截图像素取样，非印象描述）

| 元素 | 取样结论 |
|---|---|
| 页面底色 | 浅蓝灰 / 淡紫（≈ `#e9ecf7`），带极淡冷色光晕 |
| 内容面板 | 纯白、大圆角（16–24px）、几乎无描边、极轻投影 |
| 主按钮 | 宝石蓝（取样 ≈ `#2964bf`～`#3067bc`），胶囊形，白字 |
| 导航选中项 | **深墨色胶囊 + 白字**（不是下划线、不是主色填充） |
| 分段控件（进行中 / 已打开） | 浅灰胶囊容器 + 选中项深墨胶囊 |
| 状态标签 | 淡底 + 同色字药丸（人力资源＝琥珀、不合适＝红、已入选＝绿） |
| 表格 | 表头无底色、11–12px 次级灰；行分隔线极淡；行高 44–56px |
| KPI 卡片 | 图标方块（淡蓝底）+ 小号标签 + 大号数字（22px 级）+ 口径说明 |
| 过滤器 / 输入 | 全部胶囊化（含搜索框），白底描边 |

### 2.3 代码研究小结（动手前已核实）

- **文件 / 模块**：`styles/globals.css`（令牌唯一载体）、`shared/ui/components/*`（23 个基础组件）、
  `widgets/layout/*`（顶栏 / 侧边栏 / 页面骨架 / 列表四件套）、`pages/dashboard-page.tsx`、
  `features/iam/pages|components/*`、`features/portal/*`。
- **已确认事实**：
  1. 页面与 feature **只从 `@/shared/ui` 取 UI 组件**（红线），因此改基础组件＝全站生效；
  2. 列表页几乎没有各写各的表格——都走 `PageShell + SummaryStrip + ListToolbar + ResourceTable`，
     因此"列表页重做"等于"改这四件套 + 清扫少数手写面板"；
  3. `.auth-scope` 通过**重映射语义令牌**让登录页与主站共用组件但配色相反，重构必须保持该机制；
  4. 顶栏已有平台级导航，工作区用左侧栏；`AGENTS.md §7.4` 中"桌面端不设顶部导航栏 + 全局入口收敛到侧边栏底部"
     与当前代码不一致（引用的 `sidebar-footer.tsx` / `mobile-nav.tsx` 不存在）——见 §8 遗留项。
- **约束**：i18n 中英词条集合必须一致（`i18n.test.ts`）；代码静态引用的 key 必须存在（`usage.test.ts`）；
  新增交互组件必须带回归测试；不留未接线的死代码。

### 2.4 参考视觉里**不能照搬**的部分

参考视觉含"同比 +20%""5 分钟之前更新""通知中心"等元素。当前后端没有历史快照 / 变更订阅 /
通知数据源，**照做就只能编数据或放死按钮**（违反"配置即生效 / 不留死代码"两条既有教训）。
本轮据此明确取舍：只搬**视觉语法**（材质、层级、形态），不搬**没有数据支撑的装饰**。

## 3. 决策记录

- **D1 令牌先行**：先定"画布 / 面板 / 反相"三层材质与固定档位的圆角、投影，再改组件。理由是
  逐个页面调样式必然漂移（历史教训：同一项目里 `rounded-lg` / `rounded-xl` / `rounded-md` 混用）。
- **D2 用反相墨色表达"我在哪"**：导航选中项与分段控件选中项＝`bg-ink`（浅色下深墨、暗色下近白）。
  主色因此可以专用于动作，页面上不会出现两处抢眼的蓝。
- **D3 状态色一律"淡底 + 同色字"**：`Badge` / `StatusPill` 不再使用实心色块（实心红绿会把表格压得很吵）。
- **D4 列表页 KPI 化**：`SummaryStrip` 由"细长概况条"升级为 **KPI 卡片行**（图标方块 + 大数字 + 口径），
  并新增 `variant="inline"` 供弹窗 / 卡片内部使用。一次改动让 8 个列表页同时对齐参考视觉。
- **D5 顶栏搜索做成真功能**：参考视觉的全局搜索框不做"摆设"，实现为**全局快速跳转面板**
  （拍平两级导航 + 复用 `can()` 门禁）。宁可少一个装饰，也不要一个点了没反应的框。
- **D6 认证页保持克制**：登录 / OAuth / 注册页继续走 `.auth-scope`（白场 / 近黑舞台 + 墨黑主按钮），
  只跟随新的圆角与控件形态，不加品牌氛围。

## 4. 功能需求

### FR-1 设计令牌（框架）

- `globals.css` 重写浅 / 暗双值：新增 `--canvas`、`--ink`、`--ink-foreground`、`--canvas-glow-a/b`；
  投影改为 `--elevation-*` 经 `@theme` 映射为 `shadow-soft / shadow-lifted / shadow-pop`。
- 圆角刻度固定为 `sm 8 / md 10 / lg 12 / xl 18 / 2xl 24 / 3xl 32`；`.app-canvas` 提供画布底色 + 两处冷色光晕；
  补充细滚动条样式。

### FR-2 外壳与骨架（框架）

- `AppLayout` 挂 `.app-canvas`；`TopBar` 改为**悬浮玻璃岛**（不贴边、`rounded-2xl`、
  `bg-card/60 + backdrop-blur-2xl + backdrop-saturate-150 + shadow-soft`，高 64px）：
  品牌 → 胶囊导航（选中 `bg-ink`）→ 快速跳转 → 账号菜单；窄屏平台导航折叠为**同一玻璃岛内的第二行**，
  工作区导航收进抽屉；用户门户 `PortalLayout` 复用同一外形。
- `AppSidebar` 改为画布上的白色圆角浮层（`rounded-2xl + shadow-soft`），组织切换器与导航项胶囊化。
- `PageShell` 标题升到 26px、描述 13px，板块间距统一。
- `SecuritySubnav` 与门户（`PortalLayout`）同步为同一套胶囊导航。

### FR-3 全局快速跳转（框架，新能力）

- 顶栏搜索入口 + 面板：输入过滤、上下键移动、回车跳转、Esc 关闭、按"平台级 / 当前组织"分组；
  无当前组织时只列平台级目标；无读权限的组织资源不出现。

### FR-4 基础组件（按钮 / 卡片 / 表单控件）

- `Button`：全胶囊，`default h-10 / sm h-8 / lg h-11 / icon size-9`；`outline` 为白底描边（"导出"位）；
  `secondary` 去投影。
- `Card` / `FormSection`：`rounded-2xl + border-border/70 + shadow-soft`，内边距收到 `px-5 / py-5`。
- `Input` / `Textarea` / `Select` / `MultiSelect` / `Combobox` / `TagInput`：`h-10`、`rounded-xl`、
  白底、`ring-4 ring-primary/25` 焦点环。

### FR-5 基础组件（反馈与浮层）

- `Badge` / `StatusPill`：全胶囊、淡底同色字。
- `Table`：表头 11px 次级灰无底色、行高 56px、分隔线 `border-border/60`、悬停 `bg-accent/50`。
- `Tabs`：浅灰胶囊容器 + 反相墨色选中项。
- `Dialog` / `DropdownMenu` / `SelectContent` / `Tooltip`：`rounded-xl/2xl + shadow-pop`，遮罩墨色轻模糊。
- `EmptyState` / `Skeleton` / `LoadError` / `SwitchField`：与面板圆角、配色对齐。

### FR-6 列表页四件套（功能页面）

- `SummaryStrip` → KPI 卡片行（`icon` / `hint` / `to` 可点）；`ListToolbar` 搜索与筛选胶囊化；
  `ListPagination` 当前页为墨色胶囊；`ResourceTable` 容器升为 `rounded-2xl + shadow-soft`。

### FR-7 平台总览页（功能页面）

- 页头改为**问候语 + 日期**（`dashboard.greeting` / `formatLongDate`），右侧放主行动（新建应用）。
- 8 项规模指标升为 KPI 卡片行，**每张卡即入口**（点击进入对应列表）。
- 最近操作列表改为"图标方块 + 动作 + 操作者 · 组织 + 相对时间"。
- 修复：`syncers` 卡片原先指向 `/institutions/:id/syncers`（**不存在的路由**），改为 `/connections`。

### FR-8 其余页面的面板清扫（功能页面）

- 把页面里手写的 `rounded-lg/xl border` 面板统一为 `rounded-2xl + border-border/70 + shadow-soft`，
  卡片内的次级容器降为 `rounded-xl` 且不带投影（避免"盒子套盒子都浮起来"）。
- 涉及：机构 / 提供商 / 身份连接 / 应用 / 会话 / 安全 / 任务中心 / 门户各页与连接目录、连接卡片等组件。

### FR-9 顺手修复的两处真实缺陷（功能页面）

两处都是"页面看起来正常、点下去才发现"的类型，重构总览页时必须一并清掉：

1. **身份连接入口指向不存在的路由**：KPI 卡片原先指向 `/institutions/:id/syncers`，而该路径已废弃
   （身份连接统一在 `/connections`），点击后工作区渲染空白。→ 改为 `/connections`。
2. **组织聚合表把行 key 与链接拼成 `undefined`**：`/institutions/stats` 实际下发的是**组织标识**
   （驼峰 `institutionID`，如 `built-in`），而前端类型声明为 `institution_id: number`，导致
   `key={undefined}`（React 报 "unique key" 警告）与 `/institutions/undefined` 死链。
   → 前端类型按**真实下行**修正，并用组织列表把标识翻成主键 id 再拼链接（翻不到时回退标识，
   由 `WorkspaceLayout` 的兼容重定向兜底）。截图核对：表内链接为 `/institutions/1`、`/institutions/2`，
   控制台零警告。

### FR-10 系统 Logo 重做 + 规范对齐（2026-10-15 追加）

- **标记**（`shared/components/brand-mark.tsx`）：由"靛蓝圆角方块 + 北极星"改为
  **光球 + 北极星**——径向渐变冷边（青 → 靛）裹住暖芯（橙 → 琥珀），白星压在球上、
  星心留孔透出最亮的琥珀核心（既对齐参考图 Global 的"发光球体"质感，又保住北极星语义）。
  `public/icon.svg` / `public/favicon.svg` / `index.html` 的 `theme-color` 同步。
- **字标**（新增 `shared/components/brand-lockup.tsx`）：平台名用「**光球替换字母 `o`**」写法
  （`P`＋球＋`laris`，与参考图同构），顶栏 / 门户 / 登录页页眉统一用它；
  传入组织 Logo 时自动切换为「组织 Logo + 名称」，不破坏品牌作用域规则。
  新增 `brand-lockup.test.tsx` 锁定这两种形态与"控制台不出现组织品牌"的边界。
- **规范对齐**：`AGENTS.md §7.4` 的「设计语言」与「框架布局」两块按实现改写
  （宝石蓝主色 + 画布 / 面板 / 反相墨色三层材质；顶栏＝悬浮玻璃岛 + 工作区侧边栏；
  全局入口在顶栏账号菜单；品牌标识双组件；账号级能力在 `/portal/**`），
  移除已不存在的 `sidebar-footer.tsx` / `mobile-nav.tsx` / `profile-dialog.tsx` 等引用。

### FR-11 对齐与字标修订（2026-10-15 验收反馈）

1. **左右留白对齐**：玻璃岛原用 `px-3`，而内容区是 `px-4 sm:px-6 lg:px-8`——宽屏下菜单栏比内容缩进少 20px。
   现统一为同一套刻度，侧边栏浮层同步为 `py-5 pl-4 sm:py-6 sm:pl-6 lg:pl-8`。
   **实测验证**（1680×1050）：玻璃岛 `l=32 / r=1648`，总览 KPI 卡片 `l=32`（右侧到 1648），
   工作区侧边栏面板 `l=32 / t=102`，与内容区首行同顶同边。
2. **字标加大与上色**：字号 15/17/22 → 16/20/26，字重 semibold → **bold**，
   字母改用**蓝→紫渐变**（令牌 `--brand-wordmark-from/to`，明暗双值 + 认证作用域单独提亮），
   光球与字母间距放宽到 `0.07em`。渐变只落在字母上（`bg-clip-text`），光球仍是独立 SVG。

### FR-12 顶栏导航的弹性过渡（2026-10-15 验收反馈）

- 新增 `widgets/layout/nav-pills.tsx`：顶栏 / 门户的导航胶囊行由它统一渲染，
  选中态改为**一块滑动的墨色胶囊**（`.nav-pill-indicator`），切换时用弹性缓动跟手移动，
  而不是"每个按钮自己瞬时变色"——运动方向本身携带"我从哪来、到哪去"的信息。
- 缓动与时长进令牌：`--motion-spring: cubic-bezier(0.34,1.56,0.64,1)`、
  `--motion-pill-move: 420ms`（位移）、`--motion-pill-size: 340ms`（宽高）。
  位移与尺寸用不同时长 → 换站时滑块被"拉长再收回"，这是弹性手感的来源。
- 稳健性：首帧关过渡（避免加载时滑块从最左飞过去）；`ResizeObserver` 跟随标签宽度变化（切语言 / 字体加载）；
  位置用"两者 rect 之差 + `scrollLeft`"（亚像素精度 + 窄屏横滑也算得对）；
  `prefers-reduced-motion` 下关闭过渡。
- 实测（真实后端 + headless Chrome）：滑块与选中项 `l=357 / w=62` 完全重合；
  点击 "Roles" 后 110ms 采样 `translateX=96.3px`（目标 205.375px）——**确实处于动画中而非瞬移**。
- 回归：`nav-pills.test.tsx` 锁定"唯一滑块 + 选中项不自己画墨底 + 无选中项时不渲染滑块"。

## 5. 设计

### 5.1 令牌（浅色 / 暗色）

| 令牌 | 浅色 | 暗色 |
|---|---|---|
| `--canvas`（= `--background`） | `oklch(0.941 0.015 265)` | `oklch(0.168 0.013 265)` |
| `--card` | `oklch(1 0 0)` | `oklch(0.218 0.015 265)` |
| `--foreground` | `oklch(0.285 0.03 265)` | `oklch(0.955 0.006 265)` |
| `--muted-foreground` | `oklch(0.585 0.02 265)` | `oklch(0.705 0.015 265)` |
| `--primary` | `oklch(0.535 0.15 259)` | `oklch(0.7 0.135 259)` |
| `--ink` / `--ink-foreground` | `oklch(0.3 0.028 265)` / 近白 | 近白 / `oklch(0.18 0.015 265)` |
| `--border` | `oklch(0.925 0.01 265)` | `oklch(1 0 0 / 10%)` |
| success / warning / info | `0.6 0.13 165` / `0.635 0.145 55` / `0.6 0.14 252` | 各提亮一档 |

### 5.2 组件清单（新增 / 改造）

| 文件 | 变更 |
|---|---|
| `styles/globals.css` | 令牌重写 + `.app-canvas` + 滚动条 |
| `widgets/layout/nav-search.tsx`（新） | 全局快速跳转 |
| `widgets/layout/nav-search.test.tsx`（新） | 过滤 + 跳转 / 权限门禁 / 无组织 三条用例 |
| `shared/lib/long-date.ts`（新）+ 测试 | `formatLongDate`、`greetingPeriodKey` |
| `widgets/layout/{top-bar,app-sidebar,nav-list,page-shell,summary-strip,list-toolbar,list-pagination,account-menu,institution-switcher,app-layout}.tsx` | 框架改造 |
| `shared/ui/components/*`（12 个） | 按钮 / 卡片 / 徽章 / 输入 / 文本域 / 下拉 / 表格 / 分段 / 弹窗 / 提示 / 空态 / 骨架 / 多选 |
| `shared/components/{status-pill,form-section,switch-field,load-error,tag-input}.tsx` | 形态对齐 |
| `pages/dashboard-page.tsx` | 问候 + KPI + 面板重排 + 死链修复 |
| `features/iam/{pages,components}/*`、`features/portal/*` | 面板圆角 / 投影 / 导航清扫 |
| `vitest.setup.mjs` | 补 `Element.prototype.scrollIntoView` 桩（jsdom 缺失，面板一渲染即抛错） |

## 6. 实施计划（三步）

| 步 | 范围 | 状态 |
|---|---|---|
| ① 框架 UI | 令牌、画布、顶栏、侧边栏、页面骨架、快速跳转、工具栏 / 分页外壳 | 已完成 |
| ② 基础组件 UI | 按钮 / 卡片 / 徽章 / 表单控件 / 表格 / 分段 / 浮层 / 空态 / 骨架 / 状态药丸 | 已完成 |
| ③ 功能页面 UI | 总览页重排 + 8 个列表页（随四件套自动对齐）+ 12 处手写面板清扫 | 已完成 |

## 7. 验收与回归

- `pnpm typecheck`、`pnpm vitest`、`pnpm build` 全绿（26 个测试文件 / 94 条用例）。
- **真实环境截图核对**（dev server + 真实后端 + headless Chrome，登录 admin@built-in）：
  总览 / 用户列表 / 应用列表 / 角色 / 提供商 / 身份连接 / 权限 / 用户组 / 用户详情 / 安全 /
  新建用户表单 / 应用向导 / 快速跳转面板 / 暗色两页 —— 共 15 张，逐张核对层级、圆角、间距与状态色。
- 新增回归用例：`nav-search.test.tsx`、`long-date.test.ts`；`vitest.setup.mjs` 补桩后既有 88 条用例保持通过。

## 8. 遗留与后续

1. **`AGENTS.md §7.4「框架布局」块与实现不一致**（文档称"桌面端不设顶部导航栏、全局入口收敛到侧边栏底部"，
   并引用不存在的 `sidebar-footer.tsx` / `mobile-nav.tsx`）。本轮**未改该文件**——按它开头的约定，
   改它需要先确认架构决策。当前实现是"顶部平台导航 + 工作区侧边栏 + 顶栏账号菜单"。
2. 参考视觉里的**通知中心 / 导出 / 同比趋势**没有数据源，本轮未做（见 §2.4）。
3. **后端契约**：`/institutions/stats` 的 `InstitutionStats.Institution`（`internal/service/org_stats.go`）
   json tag 为驼峰 `institutionID`，与 §3.7「JSON tag 一律蛇形」不符；且未下发组织主键 id。
   前端本轮按真实下行声明并自行做标识→id 翻译。建议下一批后端改动：tag 改 `institution`、
   补 `institution_id`（并同步冒烟断言）——属 API 契约变更，需单独一批处理。
4. `pnpm lint` / `pnpm format:test` 在仓库范围内仍有既有告警与格式差异（82 个文件），非本次引入；
   为避免污染 diff，未做全仓格式化。
