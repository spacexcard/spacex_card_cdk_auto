# CDK 管理后台 UI 全面升级方案

> 目标：解决「管理员后台太乱、用户后台一般」的问题，参考 `cardplatform` 项目的专业布局/表格/过滤体验，把 **cdk-recharge-system**（合并后的主项目）升级为统一、专业、可扩展的界面。

---

## 0. 背景与现状

**合并后主项目 = `cdk-recharge-system`**（前端 Vue3+TS+Vite，后端 Go :8080）。运营入口为 CDK 管理后台（卡台 Open API）。

**当前 CDK 前端技术栈**：Vue3 + TS + **Tailwind（自定义组件类 `card/btn-*/input`）** + 自建 `DialogHost`（toast/confirm/prompt/select）。

**现状问题**：
1. **表格是裸 HTML `<table>`**：无排序、无固定列、无加载骨架、无空态样式，列宽错乱，账号控制台一行十几个按钮挤成一团 → "太乱"。
2. **过滤器是原生 `<select>`/`<input>` 拼 flex**：无统一间距、无清除按钮、无联动。
3. **无整体布局骨架**：每个页面各自写 header + 返回链接，没有统一侧边栏/顶栏导航，页面间跳转靠 router-link 散落。
4. **用户端（客户充值/历史/仪表盘）视觉一般**：卡片堆叠，缺少品牌感与信息层级。
5. **暗色模式**：Tailwind `dark:` 有零散支持，但不统一。

---

## 1. 参考对象：`cardplatform` 的 UI 做法

**技术栈**：Vue3 + TS + **Element Plus**（`element-plus` + `@element-plus/icons-vue`）+ 自定义 CSS shell + 自建 `theme.ts`（light/dark/auto）。

**它专业在哪**（逐项对照我们缺的）：

| 维度 | cardplatform 做法 | 我们现状 |
|------|-------------------|----------|
| 表格 | `el-table`：`stripe`、`v-loading`、列 `sortable="custom"`、固定列、`#default="{row}"` 自定义单元格、`el-tag` 状态、`el-tooltip/el-popover` 详情 | 裸 table，无以上任何能力 |
| 过滤/工具栏 | `.toolbar`：`共X条` + `el-input`(带 `prefix` 搜索图标/`clearable`) + `el-select` + `el-switch` + 刷新按钮，统一右对齐 | 原生控件 flex 堆叠 |
| 布局 | `app-shell` + `topnav`（品牌 + `nav-pills` 胶囊导航 + `el-dropdown` 更多菜单 + 主题/语言/余额 chips/消息铃铛） | 无统一骨架 |
| 分页 | `el-pagination` | 手写 `« ‹ › »` |
| 弹窗 | `el-dialog` + `el-form`（`el-form-item` 校验） | 已有自建 DialogHost（可保留） |
| 反馈 | `ElMessage`/`ElMessageBox` | 已有自建 toast（可保留） |
| 主题 | `theme.ts`：`documentElement.classList.toggle('dark')`，light/dark/auto 持久化 | Tailwind dark 零散 |
| 状态标签 | `el-tag` type=success/warning/danger/info + 自定义 legend/supreme tag | 手写 span+背景色 |

**结论**：cardplatform 的专业感 ≈ **Element Plus 组件库** + **统一布局骨架** + **主题系统**。这三样正是我们要引入的。

---

## 2. 升级总方针

**采用 Element Plus 作为管理后台的组件底座**（数据密集页收益最大），保留已有的 `ext.ts`(反代封装) 与业务逻辑，替换"表现层"。分阶段、可回滚。

- **数据密集的 admin 页**（账号控制台 / 卡台 / 代理 / CDK管理 / 充值审核）→ 全面用 `el-table` + `el-form` 过滤 + `el-pagination` + `el-tag` + `el-dialog`。
- **整体加一层布局骨架** `AdminLayout.vue`：左侧 `el-menu` 侧边栏（分组：账号/卡台/代理/CDK/审核/统计/审计）+ 顶栏（品牌、主题切换、语言、当前管理员、退出）。admin 路由套 children。
- **用户端**（充值/历史/仪表盘）：轻量升级，用 Element Plus 的卡片/步骤条/描述列表提升层级，不必大改。
- **主题**：引入 `theme.ts`（light/dark/auto），Element Plus 暗色 + 我们的 CSS 变量统一。
- **DialogHost / dialog.ts 保留**，或逐步用 `ElMessage/ElMessageBox` 替代。

> 备选（不推荐）：不引入 Element Plus，纯 Tailwind 手写一套 DataTable/Filter/Layout 组件。工作量不比引入 Element Plus 小，且长期维护更累、专业度不如成熟库。

---

## 3. 设计规范（Design Tokens）

统一到一套变量（`src/style.css` + Element Plus 主题覆盖）：

- **主色**：`--brand: #6366f1`（靛蓝，延续现有 btn-primary）；成功 `#059669`、警告 `#d97706`、危险 `#dc2626`、信息 `#2563eb`。
- **圆角**：卡片 `12px`、按钮/输入 `8px`、标签 `6px`。
- **间距**：页面内容 `max-w` 容器 + `24px` padding；区块间距 `20px`；工具栏内 `12px`。
- **字号**：标题 `text-2xl/3xl`、区块标题 `text-xl`、正文 `14px`、次要 `12px`。
- **暗色**：`.dark` 下背景 `#0f172a/#1e293b`、文字 `#e2e8f0`、边框 `rgba(255,255,255,.1)`。
- Element Plus 通过 CSS 变量覆盖 `--el-color-primary` 等对齐主色。

---

## 4. 组件蓝图（新建到 `src/components/ui/` 或直接用 el-*）

| 组件 | 用途 | 基于 |
|------|------|------|
| `AdminLayout.vue` | 侧边栏 + 顶栏骨架，admin 路由容器 | el-container/el-aside/el-menu/el-header |
| `DataTable`（薄封装，可选） | 统一 loading/空态/分页的表格 | el-table + el-pagination |
| `FilterBar.vue` | 统一过滤工具栏（搜索+下拉+按钮，右对齐、间距一致） | el-input/el-select/el-button |
| `StatusTag`（可选） | 邮箱/GPT/卡/代理状态 → 统一 el-tag 映射 | el-tag |
| 保留 | DialogHost / dialog.ts | 现有 |

---

## 5. 分阶段实施计划

### Phase A — 基础设施（1 步到位，解锁全部）
1. `npm i element-plus @element-plus/icons-vue`
2. `main.ts` 注册 Element Plus（按需或全量）+ 图标；引入 `theme.ts` 并 `initTheme()`。
3. 定义 CSS 变量 / Element Plus 主题覆盖，对齐设计规范。
4. 新建 `AdminLayout.vue`（侧边栏 el-menu + 顶栏），改造 admin 路由为 `AdminLayout` 的 children（各 admin view 去掉自己的 header/返回链接，改由布局提供面包屑）。

### Phase B — 账号控制台（收益最大，最乱）
- 裸 table → `el-table`：勾选列(`el-table-column type=selection`)、ID、邮箱+状态(`el-tag`)、备注(可编辑 `el-input`/点击弹窗)、密码、**操作列收进 `el-dropdown` 下拉**（不再一行十几个按钮）。
- 过滤 → `FilterBar`（搜索 el-input clearable + GPT/邮箱状态/套餐 el-select + 每页 el-select）。
- 分页 → `el-pagination`（total/page-size/current-change）。
- 批量栏 → 选中后浮出 `el-button-group`，含"已选中"操作（初始化/恢复Token/Pro20x + 逐条操作）。
- 单条充值 → `el-dialog` + `el-form`（套餐/卡/方式），替代当前 dialog.select 串联（更清晰）。

### Phase C — 卡台 / 信用卡
- 卡台卡列表 → `el-table`（卡号/有效期/CVV(点开)/余额/状态 el-tag/操作下拉）。
- 开卡/充值/退款/流水 → `el-dialog` + `el-form` + `el-table`（流水）。
- 信用卡管理 → `el-table` + 展开行 `el-table-column type=expand` 显示绑定的 GPT 邮箱。

### Phase D — 代理管理
- 设置区 → `el-form`（inline，开关 el-switch、并发/间隔 el-input-number、宫屋链接 el-input + 提取按钮）。
- 代理池 → `el-table`（地址/用户名/来源 el-tag/状态/操作）+ 批量导入 `el-dialog` textarea。

### Phase E — 原有 CDK 页面统一（充值审核/CDK管理/账单/审计/统计）
- 已经是 Tailwind 卡片风；统一到 `el-table` + `AdminLayout`，视觉与 A–D 一致。

### Phase F — 用户端轻量升级
- 仪表盘/充值/历史：`el-card` + `el-steps`（充值流程）+ `el-descriptions`（订单详情）+ `el-result`（成功/失败）。品牌顶栏参考 cardplatform 的 `topnav`。

---

## 6. 关键文件改动清单

| 文件 | 改动 |
|------|------|
| `frontend/package.json` | +element-plus +@element-plus/icons-vue |
| `frontend/src/main.ts` | 注册 ElementPlus + icons + initTheme |
| `frontend/src/theme.ts`（新，移植 cardplatform） | light/dark/auto |
| `frontend/src/style.css` | 设计 tokens + el 主题覆盖 |
| `frontend/src/layouts/AdminLayout.vue`（新） | 侧栏+顶栏骨架 |
| `frontend/src/router/index.ts` | admin 路由改为 AdminLayout children |
| `frontend/src/components/FilterBar.vue`（新） | 统一过滤工具栏 |
| `frontend/src/views/admin/AccountConsole.vue` | 重构为 el-table + 操作下拉（Phase B） |
| `frontend/src/views/admin/CardPlatform.vue` | el-table + el-dialog（Phase C） |
| `frontend/src/views/admin/ProxyManagement.vue` | el-form + el-table（Phase D） |
| `frontend/src/views/admin/{CDKeyManagement,OrderReconcile,AuditLogs,AdminDashboard}.vue` | 统一 el-table + 布局 |
| `frontend/src/views/{user,Home,...}` | el-card/steps/descriptions（Phase F） |

---

## 7. 迁移注意 / 风险

- **两套样式并存期**：引入 Element Plus 后，旧 Tailwind 组件类可暂时共存，逐页迁移，避免一次性大爆炸。
- **暗色一致性**：Element Plus 暗色需引入 `element-plus/theme-chalk/dark/css-vars.css` 并配合 `.dark` 类。
- **打包体积**：Element Plus 建议按需引入（`unplugin-vue-components` + `unplugin-auto-import`）控制体积。
- **保留 dialog.ts**：业务逻辑不动，只换表现层。
- **DialogHost vs ElMessageBox**：二选一，先保留 DialogHost，Phase E 后再评估是否统一到 Element Plus。

---

## 8. 验收标准

- 所有 admin 页在统一侧栏骨架内，切换顺滑、面包屑清晰。
- 账号控制台：表格可排序、操作收进下拉、过滤统一、分页规范、加载有 loading。
- 明暗主题一键切换且全站一致。
- 用户端有品牌顶栏 + 卡片层级 + 充值步骤条。
- 移动端基本可用（Element Plus 响应式 + 侧栏可折叠）。
