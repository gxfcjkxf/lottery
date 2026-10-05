import { AdminApiError, createIdempotencyKey } from "./admin-api";

const ENDPOINT = "/api/v1/admin/rule-simulations";

export interface RuleSimulationAccount {
  super_admin: boolean;
  permissions: string[];
  permissions_by_brand?: Record<string, string[]>;
  platform_permissions?: string[];
}

export type RuleTemplate =
  "special" | "digits" | "features" | "exclude" | "attributes" | "m-select-n";
export type ConditionOperator = "equals" | "in" | "between";
export type SafeConditionField =
  | "regular_match"
  | "special_match"
  | "position_match"
  | "excluded_match"
  | "draw_sum"
  | "draw_odd_count"
  | "draw_even_count"
  | "draw_unique_count"
  | "draw_all_same"
  | "draw_first_last_same"
  | "draw_span"
  | "draw_consecutive"
  | "draw_digit"
  | "draw_parity"
  | "attribute_match";

export interface RulePool {
  min: number;
  max: number;
  values: number[];
  allow_repeat: boolean;
}

export type RuleModelName = "X_PLUS_Y" | "M_SELECT_N" | "DIGITS_0_9";

export interface RuleModel {
  model: RuleModelName;
  regular_pool: RulePool;
  special_pool: RulePool;
  regular_count: number;
  special_count: number;
  pool_size: number;
  total_count: number;
  length: number;
  allow_repeat: boolean;
  ordered: boolean;
}

export interface RuleSelectionDefinition {
  mode: "numbers" | "exclude" | "attributes" | "features";
  regular_count: number;
  special_count: number;
  exclude_count: number;
  attribute_groups: string[];
  feature_choices: Record<string, number[]>;
}

export interface RuleCondition {
  op: "all" | "any" | "not" | "equals" | "in" | "between" | "selected";
  field?: SafeConditionField;
  target?: "all" | "regular" | "special" | "digits";
  position?: number;
  attribute_group?: string;
  attribute_value?: string;
  selection_key?: string;
  value?: number;
  values?: number[];
  min?: number;
  max?: number;
  children?: RuleCondition[];
}

export interface RuleTier {
  code: string;
  condition: RuleCondition;
  odds: string;
  exclusive: boolean;
  cap_points: string | null;
}

export interface RuleDefinition {
  schema_version: number;
  model: RuleModel;
  selection: RuleSelectionDefinition;
  number_attributes: Record<string, Record<string, number[]>>;
  unit_points: string;
  prize_tiers: RuleTier[];
  mixed_tier_policy: "" | "max_all" | "max_exclusive_plus_additive";
  cap_points: string | null;
  rounding: "half_up";
  rounding_scope: "order" | "line" | "tier";
  limits: {
    max_combinations: number;
    max_multiplier: string;
    max_bet_points: string | null;
  };
}

export interface RuleTicketSelection {
  regular: number[];
  special: number[];
  digits: number[][];
  exclude: number[];
  attributes: Record<string, string[]>;
  features: Record<string, number[]>;
}

export interface RuleDraw {
  regular: number[];
  special: number[];
  digits: number[];
}

export interface RuleSimulationRequest {
  definition: RuleDefinition;
  selection: RuleTicketSelection;
  draw: RuleDraw;
  multiplier: string;
}

export interface RuleTrace {
  op: string;
  field?: string;
  actual: number;
  matched: boolean;
  children?: RuleTrace[];
}

export interface RuleTierHit {
  code: string;
  exclusive: boolean;
  matched: boolean;
  selected: boolean;
  raw_points: string;
  points: string;
  capped_points?: string;
  trace: RuleTrace;
}

export interface RuleSimulationLine {
  selection: RuleTicketSelection;
  hits: RuleTierHit[];
  points: string;
}

export interface RuleSimulation {
  won: boolean;
  normalized: RuleTicketSelection;
  combination_count: number;
  multiplier: string;
  bet_points: string;
  prize_points: string;
  raw_prize_points?: string;
  capped_prize_points?: string;
  lines: RuleSimulationLine[];
  warnings: string[];
}

export interface RuleSimulationForm {
  unitPoints: string;
  odds: string;
  multiplier: string;
  capPoints: string;
  maxBetPoints: string;
  roundingScope: "order" | "line" | "tier";
  regularNumbers: string;
  specialNumbers: string;
  drawRegular: string;
  drawSpecial: string;
  digitCandidates: [string, string, string];
  drawDigits: string;
  excludedNumbers: string;
  redNumbers: string;
  blueNumbers: string;
  selectedColors: string[];
  featureValues: Record<string, string>;
  conditionJoin: "all" | "any";
  extraConditions: Array<{
    field: SafeConditionField;
    operator: ConditionOperator;
    value: string;
    max: string;
    target: "all" | "regular" | "special" | "digits";
    position: string;
  }>;
}

export const DEFAULT_RULE_SIMULATION_FORM: RuleSimulationForm = {
  unitPoints: "1",
  odds: "35",
  multiplier: "1",
  capPoints: "",
  maxBetPoints: "",
  roundingScope: "order",
  regularNumbers: "1,2,3,4,5,6",
  specialNumbers: "7,19,31,43",
  drawRegular: "1,2,3,4,5,6",
  drawSpecial: "7",
  digitCandidates: ["3", "1", "0"],
  drawDigits: "3,1,0",
  excludedNumbers: "11,22,33",
  redNumbers: "1,3,5,7,9",
  blueNumbers: "2,4,6,8",
  selectedColors: ["red", "blue"],
  featureValues: {
    three_kind: "0",
    same_ends: "0",
    odd_count: "2",
    sum: "4",
  },
  conditionJoin: "all",
  extraConditions: [],
};

export function createRuleSimulationKeyTracker() {
  let previousInput: string | undefined;
  let previousKey = "";
  return (input: unknown) => {
    const serialized = JSON.stringify(input);
    if (serialized !== previousInput) {
      previousInput = serialized;
      previousKey = createIdempotencyKey();
    }
    return previousKey;
  };
}

export const RULE_TEMPLATE_LABELS: Record<RuleTemplate, string> = {
  special: "特别号命中",
  digits: "数字直选（三位）",
  features: "数字特征",
  exclude: "排除号码",
  attributes: "号码属性（特别号）",
  "m-select-n": "M 选 N 全中",
};

export const SAFE_CONDITION_FIELDS: SafeConditionField[] = [
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
];

const FEATURE_FIELDS: Record<
  string,
  { field: SafeConditionField; values: number[] }
> = {
  three_kind: { field: "draw_all_same", values: [0, 1] },
  same_ends: { field: "draw_first_last_same", values: [0, 1] },
  odd_count: { field: "draw_odd_count", values: [0, 1, 2, 3] },
  sum: {
    field: "draw_sum",
    values: Array.from({ length: 28 }, (_, index) => index),
  },
};

export function effectiveRuleBrandPermissions(
  account: RuleSimulationAccount,
  brandId: string,
): Set<string> {
  return new Set(
    account.permissions_by_brand === undefined
      ? (account.permissions ?? [])
      : (account.permissions_by_brand[brandId] ?? []),
  );
}

function hasPlatformSimulation(account: RuleSimulationAccount): boolean {
  return (account.platform_permissions ?? account.permissions ?? []).includes(
    "rule.simulate.platform",
  );
}

export function canSimulateRules(
  account: RuleSimulationAccount,
  brandId: string,
): boolean {
  if (account.super_admin)
    return (
      account.platform_permissions?.includes("rule.simulate.platform") ?? false
    );
  return (
    effectiveRuleBrandPermissions(account, brandId).has(
      "rule.simulate.brand",
    ) || hasPlatformSimulation(account)
  );
}

export function parseIntegerList(value: string): number[] {
  if (!value.trim()) return [];
  return value.split(",").map((part) => {
    const canonical = part.trim();
    if (!/^(0|[1-9]\d*)$/.test(canonical))
      throw new Error("请输入逗号分隔的规范非负整数。");
    const parsed = Number(canonical);
    if (!Number.isSafeInteger(parsed)) throw new Error("选号必须是安全整数。");
    return parsed;
  });
}

function conditionFor(
  field: SafeConditionField,
  value: number,
  target?: RuleCondition["target"],
): RuleCondition {
  const condition: RuleCondition = { op: "equals", field, value };
  if (conditionUsesTarget(field)) condition.target = target ?? "all";
  return condition;
}

function conditionUsesTarget(field: SafeConditionField): boolean {
  return !["regular_match", "special_match", "position_match"].includes(field);
}

function makeConditions(
  leaves: RuleCondition[],
  join: "all" | "any",
  extras: RuleSimulationForm["extraConditions"],
  model: RuleModel,
): RuleCondition {
  const safeExtras = extras.map((leaf) => {
    if (!SAFE_CONDITION_FIELDS.includes(leaf.field))
      throw new Error("条件字段不在允许范围内。");
    const value = parseIntegerList(leaf.value);
    const needsPosition = ["draw_digit", "draw_parity"].includes(leaf.field);
    if (
      (leaf.field === "position_match" || needsPosition) &&
      model.model !== "DIGITS_0_9"
    )
      throw new Error(
        "position_match/draw_digit/draw_parity 仅适用于数字模型。",
      );
    if (
      conditionUsesTarget(leaf.field) &&
      (model.model === "DIGITS_0_9"
        ? leaf.target !== "digits"
        : leaf.target === "digits")
    )
      throw new Error("条件目标与当前选号模型不兼容。");
    let position: number | undefined;
    if (needsPosition) {
      const parsedPosition = parseIntegerList(leaf.position);
      if (
        leaf.target !== "digits" ||
        parsedPosition.length !== 1 ||
        parsedPosition[0] >= model.length
      )
        throw new Error(
          "draw_digit/draw_parity 需要 digits 目标和有效位置序号。",
        );
      position = parsedPosition[0];
    }
    const context = {
      ...(conditionUsesTarget(leaf.field)
        ? { target: needsPosition ? ("digits" as const) : leaf.target }
        : {}),
      ...(position === undefined ? {} : { position }),
      ...(leaf.field === "attribute_match"
        ? { attribute_group: "color", attribute_value: "$selection" }
        : {}),
    };
    if (leaf.operator === "equals") {
      if (value.length !== 1) throw new Error("equals 条件需要一个整数值。");
      return { ...conditionFor(leaf.field, value[0], leaf.target), ...context };
    }
    if (leaf.operator === "in") {
      if (!value.length) throw new Error("in 条件至少需要一个整数值。");
      return {
        op: "in" as const,
        field: leaf.field,
        values: value,
        ...context,
      };
    }
    const upper = parseIntegerList(leaf.max);
    if (value.length !== 1 || upper.length !== 1 || value[0] > upper[0])
      throw new Error("between 条件需要有效的最小值和最大值。");
    return {
      op: "between" as const,
      field: leaf.field,
      min: value[0],
      max: upper[0],
      ...context,
    };
  });
  const children = [...leaves, ...safeExtras];
  if (children.length < 1 || children.length > 8)
    throw new Error("条件叶节点数量必须为 1 至 8 个。");
  return { op: join, children };
}

function pool(min: number, max: number): RulePool {
  return { min, max, values: [], allow_repeat: false };
}

export function buildRuleSimulationRequest(
  template: RuleTemplate,
  form: RuleSimulationForm,
): RuleSimulationRequest {
  const unitPoints = form.unitPoints;
  const odds = form.odds;
  const multiplier = form.multiplier;
  if (!/^[1-9]\d*$/.test(unitPoints))
    throw new Error("单位积分必须是规范正整数。");
  if (!isPositiveDecimalString(odds))
    throw new Error("赔率须为最多 6 位小数的正十进制数。");
  if (!/^[1-9]\d*$/.test(multiplier) || BigInt(multiplier) > 1000n)
    throw new Error("倍数须为不超过 1000 的规范正整数字符串。");
  const capPoints = form.capPoints === "" ? null : form.capPoints;
  const maxBetPoints = form.maxBetPoints === "" ? null : form.maxBetPoints;
  for (const amount of [capPoints, maxBetPoints])
    if (amount !== null && !/^[1-9]\d*$/.test(amount))
      throw new Error("封顶与投注限额须为规范正整数，或留空表示不限。");

  const selection: RuleTicketSelection = {
    regular: [],
    special: [],
    digits: [],
    exclude: [],
    attributes: {},
    features: {},
  };
  const draw: RuleDraw = { regular: [], special: [], digits: [] };
  let model: RuleModel;
  let selectionRule: RuleSelectionDefinition;
  let numberAttributes: RuleDefinition["number_attributes"] = {};
  let tierCode: string;
  let leaves: RuleCondition[];
  let conditionJoin = form.conditionJoin;

  if (template === "special") {
    selection.special = parseIntegerList(form.specialNumbers);
    draw.regular = parseIntegerList(form.drawRegular);
    draw.special = parseIntegerList(form.drawSpecial);
    model = {
      model: "X_PLUS_Y",
      regular_pool: pool(1, 49),
      special_pool: pool(1, 49),
      regular_count: 6,
      special_count: 1,
      pool_size: 0,
      total_count: 0,
      length: 0,
      allow_repeat: false,
      ordered: false,
    };
    selectionRule = {
      mode: "numbers",
      regular_count: 0,
      special_count: 1,
      exclude_count: 0,
      attribute_groups: [],
      feature_choices: {},
    };
    tierCode = "SPECIAL_MATCH";
    leaves = [conditionFor("special_match", 1)];
  } else if (template === "digits") {
    selection.digits = form.digitCandidates.map((entry) =>
      parseIntegerList(entry),
    );
    draw.digits = parseIntegerList(form.drawDigits);
    model = {
      model: "DIGITS_0_9",
      regular_pool: pool(0, 0),
      special_pool: pool(0, 0),
      regular_count: 0,
      special_count: 0,
      pool_size: 0,
      total_count: 0,
      length: 3,
      allow_repeat: true,
      ordered: true,
    };
    selectionRule = {
      mode: "numbers",
      regular_count: 0,
      special_count: 0,
      exclude_count: 0,
      attribute_groups: [],
      feature_choices: {},
    };
    tierCode = "DIGITS_FULL";
    leaves = [conditionFor("position_match", 3)];
  } else if (template === "features") {
    const featureChoices: Record<string, number[]> = {};
    for (const [feature, raw] of Object.entries(form.featureValues)) {
      const values = parseIntegerList(raw);
      if (values.length) {
        const spec = FEATURE_FIELDS[feature];
        if (!spec || values.some((value) => !spec.values.includes(value)))
          throw new Error("所选数字特征值不合法。");
        if (values.length !== 1)
          throw new Error("每项数字特征必须且只能选择一个值。");
        featureChoices[feature] = spec.values;
        selection.features[feature] = values;
      }
    }
    if (!Object.keys(featureChoices).length)
      throw new Error("至少选择一个数字特征值。");
    draw.digits = parseIntegerList(form.drawDigits);
    model = {
      model: "DIGITS_0_9",
      regular_pool: pool(0, 0),
      special_pool: pool(0, 0),
      regular_count: 0,
      special_count: 0,
      pool_size: 0,
      total_count: 0,
      length: 3,
      allow_repeat: true,
      ordered: true,
    };
    selectionRule = {
      mode: "features",
      regular_count: 0,
      special_count: 0,
      exclude_count: 0,
      attribute_groups: [],
      feature_choices: featureChoices,
    };
    tierCode = "FEATURE_MATCH";
    leaves = Object.keys(featureChoices).map((feature) => ({
      op: "selected",
      field: FEATURE_FIELDS[feature].field,
      target: "digits",
      selection_key: feature,
    }));
  } else if (template === "exclude") {
    selection.exclude = parseIntegerList(form.excludedNumbers);
    draw.regular = parseIntegerList(form.drawRegular);
    draw.special = parseIntegerList(form.drawSpecial);
    model = {
      model: "X_PLUS_Y",
      regular_pool: pool(1, 49),
      special_pool: pool(1, 49),
      regular_count: 6,
      special_count: 1,
      pool_size: 0,
      total_count: 0,
      length: 0,
      allow_repeat: false,
      ordered: false,
    };
    selectionRule = {
      mode: "exclude",
      regular_count: 0,
      special_count: 0,
      exclude_count: selection.exclude.length,
      attribute_groups: [],
      feature_choices: {},
    };
    tierCode = "EXCLUDED_CLEAR";
    leaves = [conditionFor("excluded_match", 0, "all")];
  } else if (template === "attributes") {
    const red = parseIntegerList(form.redNumbers);
    const blue = parseIntegerList(form.blueNumbers);
    numberAttributes = { color: { red, blue } };
    selection.attributes = { color: [...form.selectedColors] };
    draw.regular = parseIntegerList(form.drawRegular);
    draw.special = parseIntegerList(form.drawSpecial);
    model = {
      model: "X_PLUS_Y",
      regular_pool: pool(1, 49),
      special_pool: pool(1, 49),
      regular_count: 6,
      special_count: 1,
      pool_size: 0,
      total_count: 0,
      length: 0,
      allow_repeat: false,
      ordered: false,
    };
    selectionRule = {
      mode: "attributes",
      regular_count: 0,
      special_count: 0,
      exclude_count: 0,
      attribute_groups: ["color"],
      feature_choices: {},
    };
    tierCode = "ATTRIBUTE_MATCH";
    leaves = [
      {
        op: "between",
        field: "attribute_match",
        target: "special",
        attribute_group: "color",
        attribute_value: "$selection",
        min: 1,
        max: 49,
      },
    ];
  } else {
    selection.regular = parseIntegerList(form.regularNumbers);
    selection.special = parseIntegerList(form.specialNumbers);
    draw.regular = parseIntegerList(form.drawRegular);
    draw.special = parseIntegerList(form.drawSpecial);
    model = {
      model: "M_SELECT_N",
      regular_pool: pool(1, 49),
      special_pool: pool(1, 49),
      regular_count: 6,
      special_count: 1,
      pool_size: 49,
      total_count: 7,
      length: 0,
      allow_repeat: false,
      ordered: false,
    };
    selectionRule = {
      mode: "numbers",
      regular_count: 6,
      special_count: 1,
      exclude_count: 0,
      attribute_groups: [],
      feature_choices: {},
    };
    tierCode = "FULL_MATCH";
    leaves = [
      conditionFor("regular_match", 6, "regular"),
      conditionFor("special_match", 1, "special"),
    ];
  }

  const definition: RuleDefinition = {
    schema_version: 1,
    model,
    selection: selectionRule,
    number_attributes: numberAttributes,
    unit_points: unitPoints,
    prize_tiers: [
      {
        code: tierCode,
        condition: makeConditions(
          leaves,
          conditionJoin,
          form.extraConditions,
          model,
        ),
        odds,
        exclusive: true,
        cap_points: capPoints,
      },
    ],
    mixed_tier_policy: "max_all",
    cap_points: capPoints,
    rounding: "half_up",
    rounding_scope: form.roundingScope,
    limits: {
      max_combinations: 10000,
      max_multiplier: "1000",
      max_bet_points: maxBetPoints,
    },
  };
  return { definition, selection, draw, multiplier };
}

function isPositiveDecimalString(value: string): boolean {
  if (!/^(?:0|[1-9]\d*)(?:\.[0-9]{1,6})?$/.test(value)) return false;
  return BigInt(value.replace(".", "")) > 0n;
}

type Envelope<T> = {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
};

export function createRuleSimulationApi(fetcher: typeof fetch = fetch) {
  return {
    async simulate(
      brandId: string,
      body: RuleSimulationRequest,
      idempotencyKey = createIdempotencyKey(),
    ): Promise<RuleSimulation> {
      const response = await fetcher(ENDPOINT, {
        method: "POST",
        credentials: "same-origin",
        headers: new Headers({
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-Brand-ID": brandId,
          "Idempotency-Key": idempotencyKey,
        }),
        body: JSON.stringify(body),
      });
      let envelope: Envelope<RuleSimulation>;
      try {
        envelope = (await response.json()) as Envelope<RuleSimulation>;
      } catch {
        throw new AdminApiError(
          response.ok
            ? "Invalid server response"
            : `Request failed (${response.status})`,
          response.status,
        );
      }
      if (
        !response.ok ||
        envelope.success !== true ||
        envelope.data === undefined
      ) {
        const detail =
          typeof envelope.error === "object" && envelope.error
            ? envelope.error
            : undefined;
        throw new AdminApiError(
          typeof envelope.error === "string"
            ? envelope.error
            : (detail?.message ?? `Request failed (${response.status})`),
          response.status,
          detail?.code,
        );
      }
      return envelope.data;
    },
  };
}

export type RuleSimulationApi = ReturnType<typeof createRuleSimulationApi>;
