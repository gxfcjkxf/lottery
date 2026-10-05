import type {
  RuleCondition,
  RuleDefinition,
  RuleModel,
  RulePool,
} from "./rule-simulation-api";

const MAX_ITEMS = 10_000;
const MAX_POOL_VALUE = 1_000_000;
const MAX_COMPARISON = 20_000_000;
const MAX_INT64 = 9_223_372_036_854_775_807n;
const NAME = /^[A-Za-z][A-Za-z0-9_]{0,47}$/;
const ODDS = /^(0|[1-9][0-9]{0,9})(\.[0-9]{1,6})?$/;

function invalid(message: string): never {
  throw new Error(message);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function onlyKeys(
  value: Record<string, unknown>,
  allowed: readonly string[],
  label: string,
): void {
  const extra = Object.keys(value).find((key) => !allowed.includes(key));
  if (extra !== undefined) invalid(`${label}包含未知字段：${extra}。`);
}

function integer(value: unknown, min: number, max: number): value is number {
  return (
    typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= min &&
    value <= max
  );
}

function array(
  value: unknown,
  label: string,
  min = 0,
  max = MAX_ITEMS,
): unknown[] {
  if (!Array.isArray(value) || value.length < min || value.length > max)
    invalid(`${label}数量必须为 ${min}–${max}。`);
  return value;
}

function name(value: unknown, label: string): asserts value is string {
  if (typeof value !== "string" || !NAME.test(value))
    invalid(
      `${label}必须以 ASCII 字母开头，且仅含 ASCII 字母、数字或下划线，长度不超过 48。`,
    );
}

function poolValues(pool: RulePool): number[] {
  const values = pool.values ?? [];
  if (values.length > 0) return [...values];
  const min = pool.min ?? 0;
  const max = pool.max ?? 0;
  return Array.from({ length: max - min + 1 }, (_, i) => min + i);
}

function poolBounds(pool: RulePool): { min: number; max: number } {
  if (pool.values.length > 0)
    return { min: Math.min(...pool.values), max: Math.max(...pool.values) };
  return { min: pool.min ?? 0, max: pool.max ?? 0 };
}

function validatePool(
  value: unknown,
  label: string,
): asserts value is RulePool {
  if (!isRecord(value)) invalid(`${label}格式无效。`);
  onlyKeys(value, ["min", "max", "values", "allow_repeat"], label);
  const min = value.min ?? 0;
  const max = value.max ?? 0;
  const values = value.values ?? [];
  if (!integer(min, 0, MAX_POOL_VALUE) || !integer(max, min, MAX_POOL_VALUE))
    invalid(`${label}号码范围必须在 0–1,000,000 内且最小值不大于最大值。`);
  if (typeof value.allow_repeat !== "boolean")
    invalid(`${label}重复设置无效。`);
  if (!Array.isArray(values) || values.length > MAX_ITEMS)
    invalid(`${label}号码列表最多 10,000 个。`);
  if (values.length > 0) {
    if (min !== 0 || max !== 0) invalid(`${label}不能同时设置范围和号码列表。`);
    const seen = new Set<number>();
    for (const item of values) {
      if (!integer(item, 0, MAX_POOL_VALUE) || seen.has(item))
        invalid(`${label}号码必须唯一且在 0–1,000,000 内。`);
      seen.add(item);
    }
    return;
  }
  const size = max - min + 1;
  if (size < 1 || size > MAX_ITEMS)
    invalid(`${label}号码池大小必须为 1–10,000。`);
}

export function validateEditorModel(model: RuleModel): void {
  if (!isRecord(model)) invalid("彩种模型格式无效。");
  onlyKeys(
    model,
    [
      "model",
      "regular_pool",
      "special_pool",
      "regular_count",
      "special_count",
      "pool_size",
      "total_count",
      "length",
      "allow_repeat",
      "ordered",
    ],
    "彩种模型",
  );
  if (!["X_PLUS_Y", "M_SELECT_N", "DIGITS_0_9"].includes(model.model))
    invalid("彩种模型类型无效。");
  validatePool(model.regular_pool, "普通号码池");
  validatePool(model.special_pool, "特别号码池");
  for (const [field, value] of Object.entries({
    regular_count: model.regular_count,
    special_count: model.special_count,
  }))
    if (!integer(value, 0, 10)) invalid(`${field}必须为 0–10 的整数。`);
  for (const [field, value] of Object.entries({
    pool_size: model.pool_size,
    total_count: model.total_count,
    length: model.length,
  }))
    if (!integer(value, 0, MAX_ITEMS))
      invalid(`${field}必须为 0–10,000 的整数。`);
  if (
    typeof model.allow_repeat !== "boolean" ||
    typeof model.ordered !== "boolean"
  )
    invalid("模型的重复或有序设置无效。");

  if (model.model === "X_PLUS_Y") {
    if (
      model.regular_count + model.special_count === 0 ||
      model.pool_size !== 0 ||
      model.total_count !== 0 ||
      model.length !== 0 ||
      model.allow_repeat
    )
      invalid("X+Y 模型数量或专属字段无效。");
    for (const [pool, count, label] of [
      [model.regular_pool, model.regular_count, "普通"],
      [model.special_pool, model.special_count, "特别"],
    ] as const) {
      if (count > 0 && poolValues(pool).length === 0)
        invalid(`${label}号码池不能为空。`);
      if (!pool.allow_repeat && count > poolValues(pool).length)
        invalid(`${label}号码数量超过不重复号码池容量。`);
    }
  } else if (model.model === "M_SELECT_N") {
    if (
      model.pool_size < 1 ||
      model.pool_size > MAX_ITEMS ||
      model.total_count < 1 ||
      model.total_count >= model.pool_size ||
      model.total_count !== model.regular_count + model.special_count ||
      model.length !== 0 ||
      model.allow_repeat ||
      model.regular_pool.allow_repeat ||
      model.special_pool.allow_repeat
    )
      invalid("M 选 N 必须满足 0 < N1+N2=N < M，且普通、特别号码均不重复。");
    const regular = poolValues(model.regular_pool);
    const special = poolValues(model.special_pool);
    if (
      regular.length !== model.pool_size ||
      special.length !== model.pool_size ||
      regular.some((number) => !special.includes(number))
    )
      invalid("M 选 N 的普通池和特别池必须是相同的 M 个号码。");
  } else {
    if (
      model.length < 1 ||
      model.length > 10 ||
      !model.ordered ||
      model.regular_count !== 0 ||
      model.special_count !== 0 ||
      model.pool_size !== 0 ||
      model.total_count !== 0 ||
      !emptyPool(model.regular_pool) ||
      !emptyPool(model.special_pool)
    )
      invalid("0–9 数字模型要求 1–10 位有序数字，普通和特别号码池为空。");
  }
}

function emptyPool(pool: RulePool): boolean {
  return (
    (pool.min ?? 0) === 0 &&
    (pool.max ?? 0) === 0 &&
    (pool.values ?? []).length === 0 &&
    !pool.allow_repeat
  );
}

export function defaultRuleDefinition(model: RuleModel): RuleDefinition {
  validateEditorModel(model);
  const copiedModel = JSON.parse(JSON.stringify(model)) as RuleModel;
  const selection =
    copiedModel.model === "DIGITS_0_9"
      ? {
          mode: "numbers" as const,
          regular_count: 0,
          special_count: 0,
          exclude_count: 0,
          attribute_groups: [],
          feature_choices: {},
        }
      : {
          mode: "numbers" as const,
          regular_count: copiedModel.regular_count,
          special_count: copiedModel.special_count,
          exclude_count: 0,
          attribute_groups: [],
          feature_choices: {},
        };
  const leaves: RuleCondition[] =
    copiedModel.model === "DIGITS_0_9"
      ? [{ op: "equals", field: "position_match", value: copiedModel.length }]
      : [
          ...(copiedModel.regular_count > 0
            ? [
                {
                  op: "equals" as const,
                  field: "regular_match" as const,
                  value: copiedModel.regular_count,
                },
              ]
            : []),
          ...(copiedModel.special_count > 0
            ? [
                {
                  op: "equals" as const,
                  field: "special_match" as const,
                  value: copiedModel.special_count,
                },
              ]
            : []),
        ];
  return {
    schema_version: 1,
    model: copiedModel,
    selection,
    number_attributes: {},
    unit_points: "1",
    prize_tiers: [
      {
        code: "MAIN",
        condition:
          leaves.length === 1 ? leaves[0] : { op: "all", children: leaves },
        odds: "2",
        exclusive: true,
        cap_points: null,
      },
    ],
    mixed_tier_policy: "max_all",
    cap_points: null,
    rounding: "half_up",
    rounding_scope: "tier",
    limits: {
      max_combinations: MAX_ITEMS,
      max_multiplier: "1000",
      max_bet_points: null,
    },
  };
}

function cloneCondition(condition: RuleCondition): RuleCondition {
  return {
    ...condition,
    ...(condition.op === "in" ? { values: [...(condition.values ?? [])] } : {}),
    ...(Object.hasOwn(condition, "children")
      ? { children: (condition.children ?? []).map(cloneCondition) }
      : {}),
  };
}

/** Fill only zero values and null/omitted collections that Go's JSON tags erase. */
export function normalizeEditorDefinition(
  definition: RuleDefinition,
): RuleDefinition {
  const copy = JSON.parse(JSON.stringify(definition)) as RuleDefinition;
  const pool = (value: RuleModel["regular_pool"]) => ({
    ...value,
    min: value.min ?? 0,
    max: value.max ?? 0,
    values: [...(value.values ?? [])],
  });
  copy.model.regular_pool = pool(copy.model.regular_pool);
  copy.model.special_pool = pool(copy.model.special_pool);
  copy.selection.attribute_groups = [
    ...(copy.selection.attribute_groups ?? []),
  ];
  copy.selection.feature_choices = Object.fromEntries(
    Object.entries(copy.selection.feature_choices ?? {}).map(
      ([key, values]) => [key, [...(values ?? [])]],
    ),
  );
  copy.number_attributes = Object.fromEntries(
    Object.entries(copy.number_attributes ?? {}).map(([group, attrs]) => [
      group,
      Object.fromEntries(
        Object.entries(attrs ?? {}).map(([label, values]) => [
          label,
          [...(values ?? [])],
        ]),
      ),
    ]),
  );
  copy.prize_tiers = (copy.prize_tiers ?? []).map((tier) => ({
    ...tier,
    condition: cloneCondition(tier.condition),
  }));
  return copy;
}

export function validateEditorPoints(value: string, allowZero = false): void {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value))
    invalid("积分必须是规范的非负整数字符串，不得含小数、符号或前导零。");
  const amount = BigInt(value);
  if (amount > MAX_INT64 || (!allowZero && amount === 0n))
    invalid(
      allowZero
        ? "积分超出 int64 范围。"
        : "积分必须大于零且不超过 int64 上限。",
    );
}

export function parseEditorNumbers(
  input: string,
  maxItems = MAX_ITEMS,
): number[] {
  if (!Number.isSafeInteger(maxItems) || maxItems < 0 || maxItems > MAX_ITEMS)
    invalid("号码输入上限必须为 0–10,000 的整数。");
  const trimmed = input.trim();
  if (!trimmed) return [];
  const tokens = trimmed.split(/[\s,]+/);
  if (tokens.length > maxItems) invalid(`号码数量不能超过 ${maxItems} 个。`);
  const result: number[] = [];
  const seen = new Set<number>();
  for (const token of tokens) {
    if (!/^(0|[1-9][0-9]*)$/.test(token))
      invalid(`号码“${token}”不是规范的非负整数。`);
    const value = Number(token);
    if (!integer(value, 0, MAX_COMPARISON))
      invalid(`号码必须在 0–${MAX_COMPARISON} 内。`);
    if (seen.has(value)) invalid(`号码 ${value} 重复。`);
    seen.add(value);
    result.push(value);
  }
  return result;
}

function poolHas(pool: RulePool, value: number): boolean {
  return pool.values.length > 0
    ? pool.values.includes(value)
    : value >= pool.min && value <= pool.max;
}

function validateDefinitionSelection(definition: RuleDefinition): void {
  const { model, selection, number_attributes: attrs } = definition;
  if (!isRecord(selection)) invalid("投注方式格式无效。");
  onlyKeys(
    selection,
    [
      "mode",
      "regular_count",
      "special_count",
      "exclude_count",
      "attribute_groups",
      "feature_choices",
    ],
    "投注方式",
  );
  if (
    !integer(selection.regular_count, 0, model.regular_count) ||
    !integer(selection.special_count, 0, model.special_count) ||
    !integer(selection.exclude_count, 0, 100)
  )
    invalid("投注数量或排除数量超出模型范围。");
  const groups = array(selection.attribute_groups, "属性组", 0, 16) as string[];
  const features = selection.feature_choices;
  if (!isRecord(features)) invalid("特征选项格式无效。");
  for (const key of Object.keys(features)) name(key, "特征代码");
  switch (selection.mode) {
    case "numbers":
      if (
        selection.exclude_count !== 0 ||
        groups.length ||
        Object.keys(features).length ||
        (model.model === "DIGITS_0_9"
          ? selection.regular_count !== 0 || selection.special_count !== 0
          : selection.regular_count + selection.special_count === 0)
      )
        invalid("numbers 投注方式的数量或附加字段无效。");
      break;
    case "exclude":
      if (
        selection.exclude_count < 1 ||
        selection.regular_count ||
        selection.special_count ||
        groups.length ||
        Object.keys(features).length
      )
        invalid("exclude 投注方式的数量或附加字段无效。");
      break;
    case "attributes": {
      if (
        selection.exclude_count ||
        selection.regular_count ||
        selection.special_count ||
        groups.length < 1 ||
        Object.keys(features).length
      )
        invalid("attributes 投注方式需要 1–16 个属性组且不含其他选号字段。");
      const seen = new Set<string>();
      for (const group of groups) {
        name(group, "属性组代码");
        if (seen.has(group) || !attrs[group])
          invalid(`属性组 ${group} 不存在或重复。`);
        seen.add(group);
      }
      break;
    }
    case "features":
      if (
        selection.exclude_count ||
        selection.regular_count ||
        selection.special_count ||
        groups.length ||
        Object.keys(features).length < 1 ||
        Object.keys(features).length > 16
      )
        invalid("features 投注方式需要 1–16 个特征且不含其他选号字段。");
      for (const [key, choices] of Object.entries(features)) {
        const values = array(choices, `特征 ${key}`, 1, 100) as number[];
        uniqueIntegers(values, `特征 ${key}`, 0, MAX_COMPARISON);
      }
      break;
    default:
      invalid("投注方式必须为 numbers、exclude、attributes 或 features。");
  }
}

function uniqueIntegers(
  values: number[],
  label: string,
  min: number,
  max: number,
): void {
  const seen = new Set<number>();
  for (const value of values) {
    if (!integer(value, min, max) || seen.has(value))
      invalid(`${label}的值必须在 ${min}–${max} 内且不重复。`);
    seen.add(value);
  }
}

function validateAttributes(definition: RuleDefinition): void {
  const attrs = definition.number_attributes;
  if (!isRecord(attrs)) invalid("号码属性格式无效。");
  const groups = Object.entries(attrs);
  if (groups.length > 16) invalid("号码属性组最多 16 个。");
  const allowed = new Set([
    ...poolValues(definition.model.regular_pool),
    ...poolValues(definition.model.special_pool),
  ]);
  if (definition.model.model === "DIGITS_0_9")
    for (let i = 0; i <= 9; i++) allowed.add(i);
  for (const [group, rawLabels] of groups) {
    name(group, "属性组代码");
    if (!isRecord(rawLabels)) invalid(`属性组 ${group} 格式无效。`);
    const labels = Object.entries(rawLabels);
    if (labels.length < 1 || labels.length > 32)
      invalid(`属性组 ${group} 必须有 1–32 个标签。`);
    for (const [label, rawNumbers] of labels) {
      name(label, "属性标签");
      const numbers = array(
        rawNumbers,
        `属性 ${group}.${label}`,
        1,
        MAX_ITEMS,
      ) as number[];
      uniqueIntegers(numbers, `属性 ${group}.${label}`, 0, MAX_POOL_VALUE);
      if (numbers.some((number) => !allowed.has(number)))
        invalid(`属性 ${group}.${label} 含不属于模型号码池的号码。`);
    }
  }
}

type Domain = { min: number; max: number };

function targetDomain(
  model: RuleModel,
  target: RuleCondition["target"],
): { count: number; min: number; max: number } {
  if (model.model === "DIGITS_0_9") {
    if (target !== "all" && target !== "digits")
      invalid("数字模型的开奖范围必须为 all 或 digits。");
    return { count: model.length, min: 0, max: 9 };
  }
  if (target === "digits") invalid("非数字模型不能使用 digits 开奖范围。");
  if (target === "regular")
    return {
      count: model.regular_count,
      min: Math.min(...poolValues(model.regular_pool)),
      max: Math.max(...poolValues(model.regular_pool)),
    };
  if (target === "special")
    return {
      count: model.special_count,
      min: Math.min(...poolValues(model.special_pool)),
      max: Math.max(...poolValues(model.special_pool)),
    };
  if (target === "all") {
    const pools = [model.regular_pool, model.special_pool].filter(
      (pool, i) => [model.regular_count, model.special_count][i] > 0,
    );
    return {
      count: model.regular_count + model.special_count,
      min: Math.min(...pools.flatMap(poolValues)),
      max: Math.max(...pools.flatMap(poolValues)),
    };
  }
  invalid("条件必须指定有效的开奖范围。");
}

function conditionDomain(
  definition: RuleDefinition,
  condition: RuleCondition,
): Domain {
  const model = definition.model;
  const selection = definition.selection;
  switch (condition.field) {
    case "regular_match":
      return {
        min: 0,
        max: Math.min(selection.regular_count, model.regular_count),
      };
    case "special_match":
      return {
        min: 0,
        max: Math.min(selection.special_count, model.special_count),
      };
    case "position_match":
      return { min: 0, max: model.length };
    case "excluded_match":
      return { min: 0, max: targetDomain(model, condition.target).count };
    case "draw_sum": {
      const target = targetDomain(model, condition.target);
      if (target.count < 1) invalid("draw_sum 的开奖范围必须至少有一个号码。");
      const pools =
        model.model === "DIGITS_0_9"
          ? [{ count: model.length, min: 0, max: 9 }]
          : condition.target === "regular"
            ? [
                {
                  count: model.regular_count,
                  ...poolBounds(model.regular_pool),
                },
              ]
            : condition.target === "special"
              ? [
                  {
                    count: model.special_count,
                    ...poolBounds(model.special_pool),
                  },
                ]
              : [
                  ...(model.regular_count > 0
                    ? [
                        {
                          count: model.regular_count,
                          ...poolBounds(model.regular_pool),
                        },
                      ]
                    : []),
                  ...(model.special_count > 0
                    ? [
                        {
                          count: model.special_count,
                          ...poolBounds(model.special_pool),
                        },
                      ]
                    : []),
                ];
      const min = pools.reduce((sum, pool) => sum + pool.count * pool.min, 0);
      const max = pools.reduce((sum, pool) => sum + pool.count * pool.max, 0);
      if (max > MAX_COMPARISON)
        invalid("开奖和值可能超过 DSL 比较上限 20,000,000。");
      return { min, max };
    }
    case "draw_odd_count":
    case "draw_even_count":
    case "attribute_match":
      return { min: 0, max: targetDomain(model, condition.target).count };
    case "draw_unique_count": {
      const target = targetDomain(model, condition.target);
      if (target.count < 1)
        invalid("draw_unique_count 的开奖范围必须至少有一个号码。");
      return { min: 1, max: target.count };
    }
    case "draw_all_same":
      if (targetDomain(model, condition.target).count < 1)
        invalid("draw_all_same 的开奖范围必须至少有一个号码。");
      return { min: 0, max: 1 };
    case "draw_first_last_same":
      if (targetDomain(model, condition.target).count < 2)
        invalid("draw_first_last_same 的开奖范围必须至少有两个号码。");
      return { min: 0, max: 1 };
    case "draw_span": {
      const target = targetDomain(model, condition.target);
      if (target.count < 1) invalid("draw_span 的开奖范围必须至少有一个号码。");
      return { min: 0, max: target.max - target.min };
    }
    case "draw_consecutive": {
      const target = targetDomain(model, condition.target);
      if (target.count < 1)
        invalid("draw_consecutive 的开奖范围必须至少有一个号码。");
      return { min: 0, max: target.count - 1 };
    }
    case "draw_digit":
      return { min: 0, max: 9 };
    case "draw_parity":
      return { min: 0, max: 1 };
    default:
      invalid("条件字段无效。");
  }
}

const CONDITION_KEYS = [
  "op",
  "field",
  "target",
  "position",
  "attribute_group",
  "attribute_value",
  "selection_key",
  "value",
  "values",
  "min",
  "max",
  "children",
] as const;
const CONDITION_FIELDS = [
  "regular_match",
  "special_match",
  "position_match",
  "excluded_match",
  "draw_sum",
  "draw_odd_count",
  "draw_even_count",
  "draw_unique_count",
  "draw_all_same",
  "draw_first_last_same",
  "draw_span",
  "draw_consecutive",
  "draw_digit",
  "draw_parity",
  "attribute_match",
] as const;

function validateCondition(
  definition: RuleDefinition,
  condition: RuleCondition,
  depth: number,
  counter: { nodes: number },
): void {
  if (depth > 8) invalid("条件嵌套深度不能超过 8。");
  if (++counter.nodes > 128) invalid("每个奖级条件最多 128 个节点。");
  if (!isRecord(condition)) invalid("条件节点格式无效。");
  onlyKeys(condition, CONDITION_KEYS, "条件节点");
  // Go decodes null pointer/slice fields and empty optional strings as absent.
  // Validate that semantic spelling without mutating or rewriting the draft.
  condition = { ...condition };
  const loose = condition as unknown as Record<string, unknown>;
  for (const key of ["value", "values", "min", "max", "position", "children"])
    if (loose[key] === null) delete loose[key];
  for (const key of [
    "field",
    "target",
    "attribute_group",
    "attribute_value",
    "selection_key",
  ])
    if (loose[key] === "") delete loose[key];
  if (["all", "any", "not"].includes(condition.op)) {
    for (const key of CONDITION_KEYS.slice(1, -1))
      if (condition[key] !== undefined)
        invalid(`${condition.op} 逻辑节点不能包含叶节点参数 ${key}。`);
    const children = array(
      condition.children,
      `${condition.op} 子条件`,
      condition.op === "not" ? 1 : 1,
      condition.op === "not" ? 1 : 32,
    );
    for (const child of children)
      validateCondition(definition, child as RuleCondition, depth + 1, counter);
    return;
  }
  if (!["equals", "in", "between", "selected"].includes(condition.op))
    invalid("条件运算符无效。");
  if (
    condition.children !== undefined &&
    (!Array.isArray(condition.children) || condition.children.length !== 0)
  )
    invalid("叶条件不能包含子条件。");
  if (
    typeof condition.field !== "string" ||
    !CONDITION_FIELDS.includes(
      condition.field as (typeof CONDITION_FIELDS)[number],
    )
  )
    invalid("条件字段无效。");
  if (condition.op === "equals") {
    if (
      !integer(condition.value, 0, MAX_COMPARISON) ||
      condition.values !== undefined ||
      condition.min !== undefined ||
      condition.max !== undefined ||
      condition.selection_key !== undefined
    )
      invalid("equals 条件只能包含 0–20,000,000 的整数 value。");
  } else if (condition.op === "in") {
    if (
      condition.value !== undefined ||
      condition.min !== undefined ||
      condition.max !== undefined ||
      condition.selection_key !== undefined
    )
      invalid("in 条件只能包含 values 参数。");
    const values = array(
      condition.values,
      "in 条件 values",
      1,
      100,
    ) as number[];
    uniqueIntegers(values, "in 条件", 0, MAX_COMPARISON);
  } else if (condition.op === "between") {
    if (
      !integer(condition.min, 0, MAX_COMPARISON) ||
      !integer(condition.max, condition.min, MAX_COMPARISON) ||
      condition.value !== undefined ||
      condition.values !== undefined ||
      condition.selection_key !== undefined
    )
      invalid("between 条件只能包含有效的 min 和 max。");
  } else {
    if (
      condition.value !== undefined ||
      condition.values !== undefined ||
      condition.min !== undefined ||
      condition.max !== undefined ||
      typeof condition.selection_key !== "string" ||
      definition.selection.mode !== "features" ||
      !Object.hasOwn(
        definition.selection.feature_choices,
        condition.selection_key,
      )
    )
      invalid("selected 条件必须引用已定义的单个 feature。");
  }

  const field = condition.field;
  const digitLeaf = field === "draw_digit" || field === "draw_parity";
  if (digitLeaf) {
    if (
      condition.target !== "digits" ||
      definition.model.model !== "DIGITS_0_9" ||
      !integer(condition.position, 0, definition.model.length - 1)
    )
      invalid(
        "draw_digit/draw_parity 必须指定数字模型内的 position 和 digits target。",
      );
  } else if (condition.position !== undefined)
    invalid("此条件字段不能指定 position。");
  if (
    field === "regular_match" ||
    field === "special_match" ||
    field === "position_match"
  ) {
    if (
      condition.target !== undefined ||
      (field === "position_match") !==
        (definition.model.model === "DIGITS_0_9") ||
      definition.selection.mode !== "numbers"
    )
      invalid(`${field} 不适用于当前模型或投注方式。`);
  } else if (field === "excluded_match") {
    if (definition.selection.mode !== "exclude")
      invalid("excluded_match 仅适用于 exclude 投注方式。");
    targetDomain(definition.model, condition.target);
  } else if (!digitLeaf) targetDomain(definition.model, condition.target);

  if (field === "attribute_match") {
    if (
      typeof condition.attribute_group !== "string" ||
      typeof condition.attribute_value !== "string" ||
      !Object.hasOwn(definition.number_attributes, condition.attribute_group)
    )
      invalid("attribute_match 必须指定已定义的属性组和值。");
    const labels = definition.number_attributes[condition.attribute_group];
    if (condition.attribute_value === "$selection") {
      if (
        definition.selection.mode !== "attributes" ||
        !definition.selection.attribute_groups.includes(
          condition.attribute_group,
        )
      )
        invalid("$selection 仅可引用用户已选择的属性组。");
    } else if (
      condition.attribute_value.startsWith("$") ||
      !Object.hasOwn(labels, condition.attribute_value)
    )
      invalid("attribute_match 的属性值不存在。");
  } else if (
    condition.attribute_group !== undefined ||
    condition.attribute_value !== undefined
  )
    invalid("只有 attribute_match 可以包含属性参数。");

  const domain = conditionDomain(definition, condition);
  const possible = (value: number) =>
    value >= domain.min && value <= domain.max;
  if (condition.op === "equals" && !possible(condition.value!))
    invalid("equals 值超出该字段在当前模型下的范围。");
  if (condition.op === "in" && !condition.values!.some(possible))
    invalid("in 中没有当前模型下可用的值。");
  if (
    condition.op === "between" &&
    (condition.max! < domain.min || condition.min! > domain.max)
  )
    invalid("between 范围与字段可能值没有交集。");
  if (
    condition.op === "selected" &&
    !definition.selection.feature_choices[condition.selection_key!].some(
      possible,
    )
  )
    invalid("selected 的特征值超出字段可能范围。");
}

export function validateEditorDefinition(definition: RuleDefinition): void {
  if (!isRecord(definition)) invalid("规则定义格式无效。");
  onlyKeys(
    definition,
    [
      "schema_version",
      "model",
      "selection",
      "number_attributes",
      "unit_points",
      "prize_tiers",
      "mixed_tier_policy",
      "cap_points",
      "rounding",
      "rounding_scope",
      "limits",
    ],
    "规则定义",
  );
  if (definition.schema_version !== 1) invalid("schema_version 必须为 1。");
  validateEditorModel(definition.model);
  validateDefinitionSelection(definition);
  validateAttributes(definition);
  validateEditorPoints(definition.unit_points);
  if (definition.cap_points !== null)
    validateEditorPoints(definition.cap_points);
  if (definition.rounding !== "half_up") invalid("rounding 必须为 half_up。");
  if (!["order", "line", "tier"].includes(definition.rounding_scope))
    invalid("rounding_scope 必须为 order、line 或 tier。");
  if (!isRecord(definition.limits)) invalid("限额格式无效。");
  onlyKeys(
    definition.limits,
    ["max_combinations", "max_multiplier", "max_bet_points"],
    "限额",
  );
  if (!integer(definition.limits.max_combinations, 1, MAX_ITEMS))
    invalid("max_combinations 必须为 1–10,000。");
  validateEditorPoints(definition.limits.max_multiplier);
  if (definition.limits.max_bet_points !== null)
    validateEditorPoints(definition.limits.max_bet_points);
  if (
    !["", "max_all", "max_exclusive_plus_additive"].includes(
      definition.mixed_tier_policy,
    )
  )
    invalid("混合奖级策略无效。");

  const tiers = array(definition.prize_tiers, "奖级", 1, 32);
  const codes = new Set<string>();
  let exclusive = false;
  let additive = false;
  for (const rawTier of tiers) {
    if (!isRecord(rawTier)) invalid("奖级格式无效。");
    onlyKeys(
      rawTier,
      ["code", "condition", "odds", "exclusive", "cap_points"],
      "奖级",
    );
    name(rawTier.code, "奖级代码");
    const code = rawTier.code as string;
    if (codes.has(code)) invalid(`奖级代码 ${code} 重复。`);
    codes.add(code);
    if (
      typeof rawTier.odds !== "string" ||
      !ODDS.test(rawTier.odds) ||
      (BigInt(rawTier.odds.split(".")[0]) === 0n &&
        !/[1-9]/.test(rawTier.odds.split(".")[1] ?? ""))
    )
      invalid("赔率必须为正的精确十进制字符串，最多 10 位整数和 6 位小数。");
    if (typeof rawTier.exclusive !== "boolean")
      invalid(`奖级 ${code} 的 exclusive 必须为布尔值。`);
    if (rawTier.cap_points !== null) {
      if (typeof rawTier.cap_points !== "string")
        invalid(`奖级 ${code} 的 cap_points 必须为字符串或 null。`);
      validateEditorPoints(rawTier.cap_points);
    }
    exclusive ||= rawTier.exclusive;
    additive ||= !rawTier.exclusive;
    validateCondition(definition, rawTier.condition as RuleCondition, 1, {
      nodes: 0,
    });
  }
  if (exclusive && additive && definition.mixed_tier_policy === "")
    invalid("同时存在排他和非排他奖级时必须显式选择 mixed_tier_policy。");
}

export function editorDefinitionBytes(definition: RuleDefinition): number {
  return new TextEncoder().encode(JSON.stringify(definition)).length;
}
