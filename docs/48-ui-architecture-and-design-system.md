# 48. UI 架构与设计系统规范 (UI Architecture and Design System)

本文档定义全平台的 UI 架构、设计系统令牌（Tokens）与视觉交互规范。适用于 `user-web`（用户端）、`admin-web`（品牌后台）与 `platform-web`（平台总后台）。

在进行任何前端页面修改、重构或新增时，所有开发人员及 AI（CODEX / AGENT）必须严格遵守本规范，**禁止私自另造一套设计风格、禁止随意引入未约定的突兀颜色、禁止使用小于 11px 的不可读字号**。

---

## 1. 核心设计原则 (Design Principles)

1. **一致性与复用（Consistency & Reuse）**
   - 优先使用已在 `:root` 中声明的设计令牌（CSS 变量）和既有通用类（如 `.button`, `.panel`, `.card`, `.status-chip` 等），不随意在页面局部硬编码新的颜色值或内联样式。
2. **KISS 原则（Keep It Simple, Stupid）**
   - 遵循项目基础规则，保持清晰扁平的 CSS 架构，不为简单状态引入过度抽象的组件或重量级 CSS-in-JS 框架。
3. **清晰可读的现代层级（Legibility & Modern Hierarchy）**
   - 彻底废除旧版 6px~10px 的微型字号。正文与交互元素必须达到人机交互可读标准。
   - 卡片层次分明，采用柔和的环境环境阴影（Ambient Shadow）与干净的边框线条，避免死板脏乱的泥浊色调。
4. **严格的视口与无横向溢出标准（Zero Horizontal Overflow）**
   - 所有页面在桌面端（1440px+）、平板端（820px）和移动端（360px~620px）必须自适应，且满足：
     `document.documentElement.scrollWidth <= window.innerWidth + 1`
   - 移动端交互触控区最小高度不小于 44px。

---

## 2. 全局设计令牌 (Design Tokens)

### 2.1 调色板 (Color Palette)

系统以专业现代的高级翡翠绿（Emerald）为主品牌色，搭配曜金（Amber/Gold）作为关键焦点与特别奖励高光，辅以高清晰度的 Slate 中性色系：

| 变量名 | 默认取值 | 用途与语义 |
|---|---|---|
| `--primary` / `--brand-primary` | `#059669` | 主色调、主操作按钮、高光品牌标识、关键强调 |
| `--primary-dark` | `#047857` | 主色 Hover、激活状态、渐变深色段 |
| `--primary-light` | `#ecfdf5` | 主色浅背景、选中卡片底色、激活菜单项底色 |
| `--accent` / `--brand-accent` | `#f59e0b` | 辅助金黄色、开奖特别号码球、未读标记、公告重点 |
| `--accent-light` | `#fffbeb` | 辅助金黄色浅底色 |
| `--success` / `--brand-success` | `#10b981` | 成功提示、在线状态指示灯、正向收益 |
| `--warning` / `--brand-warning` | `#f59e0b` | 警告提示、检查中状态、限额提示 |
| `--danger` / `--brand-danger` | `#ef4444` | 危险操作、扣款负数、离线告警、排除项 |
| `--ink` | `#0f172a` | 一级主文本、标题、重点强调（Slate 900） |
| `--ink-secondary` | `#334155` | 二级文本、表单标签、导航文字（Slate 700） |
| `--muted` | `#64748b` | 次要说明、时间戳、未激活图标（Slate 500） |
| `--muted-light` | `#94a3b8` | 占位符、边角辅助文字、停用图标（Slate 400） |
| `--line` | `#e2e8f0` | 通用卡片与输入框边框（Slate 200） |
| `--line-subtle` | `#f1f5f9` | 内部轻量分割线、斑马纹行底色 |
| `--canvas` | `#f8fafc` | 全局页面背景画布（Slate 50） |
| `--surface` / `--surface-card` | `#ffffff` | 纯白卡片、浮层、抽屉与弹窗背景 |

> **禁止项**：不得在业务页面重新写回旧版泥绿色（如 `#20594c`、`#e7eee5`、`#eff3ec`、`#d9ef9b` 等脏灰绿），统一使用上述变量。

### 2.2 字号阶梯 (Typography Scale)

| 类别 | 推荐字号 | 字重 (Font Weight) | 适用场景 |
|---|---|---|---|
| **Micro / Tag** | `11px` ~ `12px` | 700 ~ 750 | 状态胶囊、号码属性小标签、微标（全大写 + 适度 letter-spacing） |
| **Caption / Meta** | `12px` ~ `13px` | 400 ~ 600 | 辅助说明、订单时间、次要说明、表单提示文案 |
| **Body (正文)** | `14px` ~ `15px` | 400 ~ 500 | 正文字段、表格行数据、输入框文字、导航项（行高 1.5~1.6） |
| **Card Title / Subhead**| `16px` ~ `20px` | 750 ~ 800 | 卡片主标题、弹窗标题、区域小节标题 |
| **Page Title** | `24px` ~ `30px` | 800 ~ 850 | 页面级一级标题 (`.page-heading h1`) |
| **Hero Display** | `32px` ~ `42px` | 850 | 首页横幅主标语、重点金额大数字 |

> **强制要求**：前端禁止出现低于 `11px` 的字号。原代码中的 `6px`, `7px`, `8px`, `9px`, `10px` 必须全部升级至上述阶梯。

### 2.3 圆角与阴影 (Radii & Shadows)

- **圆角**：
  - 小组件/标签/输入框：`--radius-sm: 8px` ~ `10px`
  - 通用卡片/面板/弹窗：`--radius: 16px`
  - 大横幅/特色容器：`--radius-lg: 20px` ~ `24px`
  - 胶囊/圆形按钮/号码球：`--radius-full: 9999px` / `50%`
- **阴影**：
  - 微阴影：`--shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.05)`
  - 卡片标准阴影：`--shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -2px rgba(0, 0, 0, 0.05)`
  - 悬浮/激活卡片：`--shadow-md: 0 10px 15px -3px rgba(0, 0, 0, 0.07), 0 4px 6px -4px rgba(0, 0, 0, 0.03)`
  - 浮层/弹窗/主横幅：`--shadow-lg: 0 20px 25px -5px rgba(0, 0, 0, 0.09), 0 8px 10px -6px rgba(0, 0, 0, 0.04)`

---

## 3. 标准组件规范 (Component Conventions)

### 3.1 彩票号码球 (Lottery Balls)
彩种选号、开奖结果及订单快照中的号码球统一具备拟真 3D 放射光泽与阴影：
- **普通号码球（翡翠色）**：
  ```css
  background: radial-gradient(circle at 35% 35%, #ecfdf5 0%, #10b981 60%, #059669 100%);
  color: #ffffff;
  border-radius: 50%;
  box-shadow: 0 4px 10px rgba(5, 150, 105, 0.35);
  font-variant-numeric: tabular-nums;
  ```
- **特别/金球（曜金色）**：
  ```css
  background: radial-gradient(circle at 35% 35%, #fffbeb 0%, #f59e0b 60%, #d97706 100%);
  color: #ffffff;
  border-radius: 50%;
  box-shadow: 0 4px 10px rgba(245, 158, 11, 0.35);
  ```
- **选号器未选中球（默认态）**：
  纯白底色、`1.5px solid var(--line)` 边框、深灰数字，Hover 时轻微放大并呈翡翠色轮廓。
- **排除号码球**：
  浅红底色（`#fef2f2`）、红色边框、删除线文本。

### 3.2 按钮体系 (Buttons)
- **主操作按钮 (`.button-primary`, `.button-dark`)**：
  翡翠渐变底色，白色文字，带翡翠色扩散阴影，Hover 时向上微移 1px 并加深阴影。
- **次级按钮 (`.button-secondary`)**：
  纯白底色，`1px solid var(--line)` 边框，Hover 时淡灰高亮，避免刺眼反差。
- **幽灵按钮 / 文字链接 (`.button-quiet`, `.text-link`)**：
  透明底，翡翠绿字，用于紧随卡片的跳转行动。

### 3.3 卡片与面板 (Cards & Panels)
- 所有列表容器、详情块使用 `.panel` 或 `.mini-panel` / `.card` 结构。
- 卡片内部采用规整的内边距（桌面端 20px~26px，移动端 14px~16px）。
- 数据标签（`<dt>`）与数值（`<dd>`）统一对齐，长哈希或长编号必须带有 `overflow-wrap: anywhere`。

### 3.4 状态标识胶囊 (Status Chips)
- 成功/已生效/正常：翡翠浅绿底 (`#ecfdf5`) + 深绿文字 (`#047857`)。
- 警告/待处理/暂停：暖金浅黄底 (`#fffbeb`) + 棕金文字 (`#b45309`)。
- 危险/已取消/异常：淡红底 (`#fef2f2`) + 深红文字 (`#b91c1c`)。
- 中性/历史归档：浅灰底 (`#f1f5f9`) + 灰度文字 (`#64748b`)。

---

## 4. 新增页面与后续迭代开发准则 (Developer Guidelines)

后续新增业务页面或对已有页面进行维护时，必须执行以下流程：

1. **结构一致**：
   - 页面顶部采用标准标题区 `.page-heading`，包含副标 `.eyebrow`、主标 `h1` 与说明文案 `p`。
   - 页面宽度使用 `.page-section`（最大宽度限制在 1100px 内，居中对齐）。
2. **样式归属**：
   - 全局及各端共用的样式统一维护在 `shared/src/styles.css` 或各前端包的 `style.css` 中。
   - 页面特有的局部样式可使用 `<style scoped>`，但变量与颜色必须继承自 `:root` 令牌，**禁止硬编码孤立颜色**。
3. **测试契约与语义化选择器保护**：
   - 现有测试（包括 Playwright E2E、契约测试、静态规范测试）所依赖的选择器（如 `.footer-online`, `.page-footer`, `.brand`, `[data-testid="..."]` 等）**必须完整保留**。
   - 国际化文案与免责声明文字不得随意修改或删除。
4. **验证门禁**：
   - 任何样式修改后，必须运行并全部通过：
     - `pnpm -r test`（全项目单元测试）
     - `pnpm test:static`（静态合规与架构检查）
     - `pnpm -r typecheck`（TypeScript 严格检查）
     - `pnpm -r build`（生产打包检查）
     - 检查移动端 360px 视口无横向滚动条。
