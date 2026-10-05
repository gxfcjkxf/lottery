# 玩法规则引擎规格

## 1. 设计目标

- 常规玩法由运营人员通过配置完成，不把每个玩法写成独立业务代码。
- 规则发布前必须校验、模拟、审核；订单保存完整规则快照或不可变版本引用。
- 历史订单永远使用下单时绑定的规则版本。
- 规则表达式不得执行任意脚本；只允许白名单组件和参数。

## 2. 三类号码模型

### 2.1 X+Y

- 普通号码池和特别号码池分开。
- 选择普通号码 X 个、特别号码 Y 个。
- `regular`、`special` 的重复、顺序和范围由配置决定。

```json
{
  "model": "X_PLUS_Y",
  "regular_pool": {"min": 1, "max": 49, "allow_repeat": false},
  "special_pool": {"min": 1, "max": 10, "allow_repeat": false},
  "regular_count": 6,
  "special_count": 1
}
```

### 2.2 M 选 N

- `M` 是非 0 正整数，表示号码池规模。
- `N < M`，总选择数为 N。
- `N = N1 + N2`；N1 是普通号码数量，N2 是特别号码数量。
- 第一阶段默认不允许重复；普通号码和特别号码的选择规则分别保存。
- 号码具体从何种业务号码池映射，由彩种配置给出，不在引擎中硬编码。

```json
{
  "model": "M_SELECT_N",
  "pool_size": 49,
  "total_count": 7,
  "regular_count": 6,
  "special_count": 1,
  "allow_repeat": false,
  "regular_pool": {"min": 1, "max": 49},
  "special_pool": {"min": 1, "max": 49}
}
```

必须校验：`M > 0`、`0 < N < M`、`N = N1 + N2`、N1/N2 非负、选择值属于对应池、重复规则满足配置。

### 2.3 0~9 N 位数字

- 数字取值固定为 `0~9`。
- N 表示有序数字位数。
- `allow_repeat=false` 时禁止同一数字出现在多个位置。
- `allow_repeat=true` 时允许 `121`、`111` 等结果。

```json
{
  "model": "DIGITS_0_9",
  "length": 3,
  "allow_repeat": true,
  "ordered": true
}
```

## 3. 规则定义结构

每个玩法规则版本至少包含：

```json
{
  "version": 3,
  "model": "DIGITS_0_9",
  "selection": {},
  "features": [],
  "constraints": [],
  "combination": {},
  "prize_tiers": [],
  "limits": {},
  "cancellation": {},
  "effective": {}
}
```

### 组件白名单

- `number_pool`
- `count`
- `position`
- `attribute`
- `exclude`
- `odd_even`
- `sum`
- `range_size`
- `repeat_pattern`
- `same_position`
- `consecutive`
- `combination`
- `multiplier`
- `prize_tier`
- `cap`
- `rounding`

条件节点使用 `all`、`any`、`not`、`equals`、`in`、`between` 等固定操作；不允许配置代码字符串并动态执行。

## 4. 投注计算

1. 读取当前期次和玩法有效版本。
2. 规范化用户选号，保留原始输入和规范化结果。
3. 执行号码范围、重复、位置、排除和附加属性校验。
4. 计算复式展开组合数量。
5. 计算基础积分：`unit_points × combination_count × multiplier`。
6. 应用单注、单期、用户和玩法限额。
7. 生成扣款来源分配。

复式和倍投使用传统算法；第一阶段不允许自定义脚本改变计算过程。

## 5. 中奖与派奖计算

- 结算输入：规则版本、注单规范化内容、开奖结果、开奖结果特征、倍数。
- 输出：是否中奖、命中奖级、基础积分、封顶后积分、四舍五入后积分、解释明细。
- 大多数玩法一个奖级；多个奖级由 `exclusive` 配置决定。
- 排他奖级按封顶后的中奖金额选择一个，金额相同按定义中的稳定顺序选择；不能用 priority 选择较低金额。非排他奖级累加。该口径以用户原始要求为准，修正早先 priority 文案。
- 四舍五入精度和封顶值读取规则配置；积分最终必须是整数。
- 中奖积分写入可用积分中的 `winning_points` 来源。

结算输出示例：

```json
{
  "won": true,
  "tiers": [{"code": "SPECIAL_MATCH", "exclusive": true, "priority": 10}],
  "base_points": 100,
  "capped_points": 100,
  "rounded_points": 100,
  "explanation": {"special_match": true}
}
```

## 6. 规则生命周期

立即模式：`draft → pending_review → active → expired/rolled_back`；下期模式：`draft → pending_review → approved → active → expired/rolled_back`。审核驳回进入 `rejected`。

- 创建者及所有成功更新过该版本草稿的编辑者均不能审核（通过或驳回）。
- 具有对应权限的品牌管理员处理本品牌规则；平台超级管理员只查看，全部彩种/玩法版本工作流写入均拒绝。
- 生效模式：立即生效或下期生效。
- S4-b 已保存版本历史和开期版本绑定；用户投注与历史订单绑定仍是后续阶段契约。
- 回滚通过克隆旧定义创建新草稿，重新验证和审核，不删除历史或直接切换旧版本。

## 7. 配置校验与模拟

提交审核前必须自动执行：

- 号码范围和数量校验；
- `M/N/N1/N2` 约束校验；
- 重复、排除和条件冲突校验；
- 奖级不可达、奖级重叠和排他性校验；
- 赔率、封顶、舍入和限额边界校验；
- 至少一组测试选号 + 测试开奖结果 + 预期结算结果。

后台必须能模拟指定选号和开奖结果，并展示每个条件节点、命中奖级和最终积分。

上述为完整设计目标；S4-b 的静态证明有明确边界，只阻断可证明的矛盾/不可达并提示已发现的重叠，不宣称任意复合条件完整覆盖（见 §9）。

## 8. S4-a 可执行 DSL v1（实际实现）

前面的概念字段是完整平台设计；本节为可直接交给实现方和 API 使用的计算结构。生命周期版本与生效信息已由 S4-b 业务版本记录保存，取消业务仍待后续实现，不能把 `effective`、`cancellation` 或代码字符串直接塞入计算 Definition。

计算入口 `SimulateContext(ctx, SimulationInput)`；当前仅模拟，不扣款、不创建订单、不发布规则。请求含 definition、selection、draw、multiplier（规范正整数字符串）。定义采用闭合 JSON schema，所有深度拒绝重复键、未知字段；无脚本、eval 或动态程序执行。

### 定义字段

- schema_version：固定 1。
- model：本文件三类 Model 对象；X+Y 支持两独立池、重复和有序设置。M 选 N 的两池必须是同一组 M 个号码，普通/特别号之间也不重复；不同池用 X+Y。数字 N 位的 ordered 必须为 true，0~9 是隐含范围，普通/特别池为空。
- selection：mode 为 numbers/exclude/attributes/features。numbers 的 regular_count/special_count 是**玩法投注数量**，可小于完整开奖数量，例如只买特别号设置 0+1。数字选号为每个位置的候选数组，支持复式、重复数字与开头的零。
- number_attributes：`{组代码:{属性代码:[号码...]}}`，同一号码可同时对应多个属性。attributes 模式指定 attribute_groups，选号每组可选多值；展开后每组一个值。features 模式指定 feature_choices（允许值字典），每组候选按笛卡尔积展开。
- unit_points、limits.max_multiplier、可选 cap_points、limits.max_bet_points：规范整数字符串；unit/max_multiplier 必须大于零，cap/max_bet 为正值或 null。
- limits.max_combinations：1–10000。普通无序选号按组合展开，有序按排列，允许重复时支持重复组合/排列；M 选 N 过滤普通/特别跨组重复，数字禁止重复时过滤非法单线。没有合法单线拒绝。
- prize_tiers：1–32 个唯一代码的奖级，每项含 condition、odds（正十进制字符串，最多六位小数）、exclusive、cap_points（正整数或 null）。赔率不是浮点数。
- rounding 固定 half_up；rounding_scope 必须显式为 order/line/tier，分别在整注汇总、每个组合、每个奖级执行整数四舍五入。无推断默认值，规则审核必须查看该字段。
- 金额比较使用该舍入层级下的实际精确值；order/line 不提前按奖级舍入。整数 points 仅为独立舍入展示，不能取代 raw/capped 值进行奖级选择。
- 全排他奖级选择金额最高者；全非排他累加。混合时 mixed_tier_policy 必须显式选择 max_all（有排他命中则全部命中取最高）或 max_exclusive_plus_additive（排他中取最高，再加非排他）；不存在未声明的混合规则。

### 条件结构与字段

逻辑节点 all/any 含 1–32 个 children，not 恰好一个 child；不得附加叶节点参数。叶节点只允许 equals（value）、in（values）、between（min/max，含端点）、selected（selection_key），且只能使用该运算所需参数。selected 对照展开后用户 features 的单一选择值。

| 字段 | 值与适用范围 |
|---|---|
| regular_match / special_match | 投注与开奖普通/特别号码的多重集交集数量，重复按实际次数匹配；不含 target/position |
| position_match | N 位数字逐位相同的数量；不含 target/position |
| excluded_match | 被排除的号码在指定开奖结果部分中出现的次数 |
| draw_sum / draw_odd_count / draw_even_count | 指定部分和值、奇数个数、偶数个数 |
| draw_unique_count / draw_all_same / draw_first_last_same | 不同号码个数；全部相同/首尾相同取 0 或 1 |
| draw_span / draw_consecutive | 最大值减最小值；排序后相邻数差为 1 的邻接对数，不将重复号码算为连续 |
| draw_digit / draw_parity | 指定数字位置的值 / 奇偶（偶=0、奇=1）；target=digits、position 从 0 开始 |
| attribute_match | 指定部分中命中属性的号码次数；attribute_group 与字面属性值或 `$selection`，每个开奖结果位置最多计一次 |

开奖结果部分 target 为 all/regular/special/digits；必须符合模型。属性字面值可用于普通号码玩法；`$selection` 仅使用该玩法允许用户选择的属性组。叶节点会检查字段值域，明显不可达的比较被拒绝；这不等于已经证明所有复合条件无矛盾。

### 输出、精度与防滥用

输出含 normalized、combination_count、bet_points、prize_points、won、lines、warnings。每个组合含全部奖级的 matched/selected、精确 raw_points/capped_points、整数 points 和完整条件 Trace；非短路展示所有条件。顶层 raw_prize_points/capped_prize_points 解释最终汇总和封顶。

中间值使用任意精度有理数，展示的中间整数也是字符串，不提前把大金额转为 int64。最终投注额与派奖必须在 int64 范围；总封顶在最终整数溢出检查前生效。order 模式下各组合独立舍入的展示值不能替代整注的精确汇总；中奖条件命中但舍入为零仍保持 won=true。

每个条件深度最多 8、节点最多 128；一次模拟解释节点总量最多 200000。号码池最多 10000，号码值 0–1000000、每组数量最多 10、数字长度 1–10。跨组不重复可行性使用集合约束，数字位置不重复使用二分图匹配，不能穷举无解排列。执行检查请求取消，组合工作量另有上界。HTTP 请求沿用后台 16 KiB 上限。

模拟返回风险提示；商业赔率合理性、任意复合条件的完整可达性和覆盖率仍需案例覆盖及品牌管理员审核。独立 S4-a 模拟不发布规则；S4-b 的已保存版本验证和审核见下节，自动期次调度、开奖和投注结算仍待后续阶段。

## 9. S4-b 已保存规则版本与验证审核（实际实现）

### 品牌目录、权限与版本

彩种保存合法 Model 与时区，玩法归属该彩种和品牌；草稿只能使用通过 DSL 校验且与所属彩种一致的完整 Model（不只比较模型枚举）。三类模型枚举为 `X_PLUS_Y`、`M_SELECT_N`、`DIGITS_0_9`。请求与响应见 [API 契约 S4-b](04-api-contract.md#s4-b-已接入彩种玩法与规则版本工作流)。

- 彩种/玩法目录读取：`game.view.brand` 或 `game.view.platform`；创建：仅 `game.write.brand`。
- 规则读取：`rule.view.brand` 或 `rule.view.platform`；创建/修改/克隆：`rule.write.brand`；验证：`rule.validate.brand`；送审：`rule.submit.brand`；审核通过/驳回：`rule.review.brand`。
- 权限精确匹配，不互相隐含；平台读取也必须选择品牌。没有平台写入权限路径，超级管理员即使误配品牌写权限，所有上述写入仍拒绝。独立模拟的 `rule.simulate.platform` 不授予版本工作流写入权。

每个玩法的 `version_no` 是创建时分配、永不修改的历史序号；`version` 是记录乐观锁版本，更新、验证和状态变更都会递增。后续请求使用最新返回的 `version`，不能拿 `version_no` 作为锁版本。仅 `draft` 可修改 Definition 和生效模式；保存草稿会清除旧验证报告。送审后 Definition、生效模式和验证证据不可改写，数据库也禁止删除历史及改写审核证据。

### 验证报告与审核隔离

验证针对已保存的 Definition，接受 1–32 个用例。每例有非空、去空白后唯一的 name（最多 120 UTF-8 字节）、selection、draw、正整数 multiplier，以及必填的 expected_bet_points、expected_prize_points、expected_won。预期积分是规范非负整数字符串；不要求发送模型不适用的空选号/开奖字段。整个用例集共享最多 200000 个解释节点的执行预算。

报告包含 `passed`、`definition_hash`、`cases`、`warnings`、`findings`。hash 是确定性序列化的类型化 Definition 的 SHA-256（不是任意原始 JSON 的字节摘要）。每个 case 保存完整原始 `input`（含 selection、draw、multiplier 和全部预期值）、预期/实际 bet/prize/won、`matched` 与完整 `simulation` 输出及各条件 Trace；积分精确比较，不转浮点。案例结果不符时验证请求仍成功保存报告，但 `passed=false`；非法输入或超执行预算则请求失败。

静态检查只做有界证明：可证明的同一标量 AND 约束矛盾、模型不变量导致的不可达奖级产生阻断 `TIER_UNREACHABLE`；重复条件或同一标量区间可证明相交产生非阻断 `TIER_OVERLAP` 警告。案例不符产生阻断 `CASE_MISMATCH`。这不是任意复合条件全覆盖或商业赔率安全证明；报告始终保留样例不能证明完整覆盖的风险提示。

送审与审核都要求报告通过且 hash 等于当前定义 hash。创建者与所有成功更新草稿的编辑者记录在不可改写的 contributors 中，均不能通过或驳回自己的版本（只改生效模式的更新也计入）；仅执行验证不自动成为贡献者。审核通过遇到 warnings 必须显式 `warnings_acknowledged=true`，原因和确认记录进入审计；驳回不要求确认警告。当前通用风险提示意味着通过审核需要确认。审核驳回是终态，不提供就地改回草稿的路径。

### 生效、队列与旧定义克隆

后台新建草稿默认选择 `immediate`，但这只是前端默认：创建、更新及 clone 请求始终必须显式发送 `effect_mode`，后端不推断默认值。`effect_mode=immediate` 在审核通过的同一事务直接激活并替换玩法 active 引用，不另调用 publish。`next_period` 审核通过后为 `approved`，绑定该彩种已实际开启序号的下一序号；每个玩法最多一个 approved 待生效版本。存在该队列时，后续通过审核（包括立即模式）返回 409 `RULE_STATE_CONFLICT`，不能覆盖队列。

下期激活由内部 `Store.OpenPeriod` 或 S4-c1 worker 对已保留 pending 期次的实际开期事务执行，两条路径共用版本绑定逻辑与彩种行锁。根据 PostgreSQL `clock_timestamp()` 要求 `bet_start_at <= now < bet_end_at`，实际开期时激活符合序号的 approved 版本，并不可变地保存该期各玩法的有效版本引用。`effective_sequence` 对应实际成功开期计数 `games.started_sequence`，不是预生成期次的创建序号；错过完整窗口的 pending 期次不会推进该计数或激活规则。不能伪造未来开期时间提前激活，也没有公开 activate-now 捷径。S4-c1 已接入日历生成及开期/截止/待开奖调度；开奖结果、投注与结算仍待后续实现。

仅 `active`、`expired`、`rolled_back` 可作为 clone 来源。clone 保存 `source_version_id`，生成新的 `version_no` 和无验证报告的 draft，必须重新验证、送审、由非贡献者审核。源克隆的 Definition 即使在 draft 也不可改写；可在草稿阶段调整生效模式。普通新版本替换旧 active 时旧版变为 expired；来源克隆版本生效时被替换的旧 active 标记 rolled_back。历史版本、审核证据和开期绑定均不删除。

S4-d 后台保留六个快捷模板，同时提供通用可视化编辑器，覆盖当前 schema-v1 的三种模型、全部选号方式、属性/特征字典、嵌套 all/any/not 与叶条件、1–32 个奖级、两种混合策略、封顶和舍入。非模板定义自动进入通用模式，不用模板猜测覆盖；模板切换只有能无损还原时才允许。模型来自当前彩种；未读目录时可明确输入一致模型，由服务器核对，不能借此改写彩种。回滚来源草稿的定义仍锁定。

验证表单支持 1–32 组独立用例，不自动计算预期值；初始零值不是通过证明。保存、用例编辑和验证结果保持分离，修改定义必须先保存并重新验证。未解析数字/CSV、节点、字典或用例错误会阻止保存或验证，结构性增删/复制不清除其他未解析输入。16 KiB 请求上限同时包含定义/用例及其他正文；DSL 项目数量上限不保证所有最大配置能同时放入一个请求。

S4-d 修正重复号码池的候选展开：允许重复的池可以用一个候选号码生成多位置重复组合；不重复池仍要求足够不同候选，M 选 N 的跨组不重复规则不变。数字位置和普通候选按服务端规则去重后判断可行性。以上配置、模拟、验证和审核均不创建投注订单、不改变积分、不派奖；案例通过不等于任意组合的完整可达性或商业赔率安全证明。
