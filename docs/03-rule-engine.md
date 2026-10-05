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
- 排他奖级按 priority 选择一个；非排他奖级累加。
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

`draft → pending_review → approved → active → expired/rolled_back`

- 创建者不能审核自己的规则。
- 品牌管理员审核本品牌规则；平台超级管理员只查看。
- 生效模式：立即生效或下期生效。
- 新版本只影响新投注；历史订单保存旧版本。
- 回滚是发布新版本指向旧定义，不删除历史版本。

## 7. 配置校验与模拟

提交审核前必须自动执行：

- 号码范围和数量校验；
- `M/N/N1/N2` 约束校验；
- 重复、排除和条件冲突校验；
- 奖级不可达、奖级重叠和排他性校验；
- 赔率、封顶、舍入和限额边界校验；
- 至少一组测试选号 + 测试开奖结果 + 预期结算结果。

后台必须能模拟指定选号和开奖结果，并展示每个条件节点、命中奖级和最终积分。
