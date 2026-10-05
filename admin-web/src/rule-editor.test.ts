import { describe, expect, it } from "vitest";
import {
  defaultRuleDefinition,
  editorDefinitionBytes,
  normalizeEditorDefinition,
  parseEditorNumbers,
  validateEditorDefinition,
  validateEditorModel,
  validateEditorPoints,
} from "./rule-editor";
import type { RuleDefinition, RuleModel } from "./rule-simulation-api";

const xPlusY: RuleModel = {
  model: "X_PLUS_Y",
  regular_pool: { min: 1, max: 35, values: [], allow_repeat: false },
  special_pool: { min: 1, max: 12, values: [], allow_repeat: true },
  regular_count: 5,
  special_count: 1,
  pool_size: 0,
  total_count: 0,
  length: 0,
  allow_repeat: false,
  ordered: false,
};

const mSelectN: RuleModel = {
  model: "M_SELECT_N",
  regular_pool: { min: 1, max: 7, values: [], allow_repeat: false },
  special_pool: { min: 1, max: 7, values: [], allow_repeat: false },
  regular_count: 2,
  special_count: 1,
  pool_size: 7,
  total_count: 3,
  length: 0,
  allow_repeat: false,
  ordered: false,
};

const digits: RuleModel = {
  model: "DIGITS_0_9",
  regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
  special_pool: { min: 0, max: 0, values: [], allow_repeat: false },
  regular_count: 0,
  special_count: 0,
  pool_size: 0,
  total_count: 0,
  length: 3,
  allow_repeat: true,
  ordered: true,
};

const validDefinition = (): RuleDefinition => defaultRuleDefinition(xPlusY);

describe("rule editor defaults and model validation", () => {
  it("builds valid full-count definitions for all three models without mutating the model", () => {
    for (const model of [xPlusY, mSelectN, digits]) {
      const before = structuredClone(model);
      const definition = defaultRuleDefinition(model);
      expect(definition.model).toEqual(before);
      expect(definition.selection.mode).toBe("numbers");
      expect(definition.limits.max_combinations).toBe(10_000);
      expect(definition.limits.max_multiplier).toBe("1000");
      expect(definition.unit_points).toBe("1");
      expect(() => validateEditorDefinition(definition)).not.toThrow();
      expect(model).toEqual(before);
    }
    expect(defaultRuleDefinition(digits).prize_tiers[0].condition).toEqual({
      op: "equals", field: "position_match", value: 3,
    });
  });

  it("allows X+Y pool repeats independently and permits one selection group to be zero", () => {
    const model = structuredClone(xPlusY);
    model.regular_count = 0;
    model.special_pool.allow_repeat = true;
    validateEditorModel(model);
    const definition = defaultRuleDefinition(model);
    expect(definition.selection.regular_count).toBe(0);
    expect(definition.prize_tiers[0].condition).toEqual({
      op: "equals", field: "special_match", value: 1,
    });
  });

  it("enforces M-select-N shared pools, N1+N2<M, and disallows repeats", () => {
    validateEditorModel(mSelectN);
    const unequal: RuleModel = { ...mSelectN, special_pool: { ...mSelectN.special_pool, max: 8 } };
    expect(() => validateEditorModel(unequal)).toThrow(/相同的 M 个号码/);
    expect(() => validateEditorModel({ ...mSelectN, total_count: 7 })).toThrow(/0 < N1\+N2=N < M/);
    expect(() => validateEditorModel({ ...mSelectN, regular_pool: { ...mSelectN.regular_pool, allow_repeat: true } })).toThrow();
  });

  it("accepts the backend M=49, N=7 (6+1) model example", () => {
    const backendExample: RuleModel = {
      model: "M_SELECT_N",
      regular_pool: { min: 1, max: 49, values: [], allow_repeat: false },
      special_pool: { min: 1, max: 49, values: [], allow_repeat: false },
      regular_count: 6,
      special_count: 1,
      pool_size: 49,
      total_count: 7,
      length: 0,
      allow_repeat: false,
      ordered: false,
    };
    expect(() => validateEditorDefinition(defaultRuleDefinition(backendExample))).not.toThrow();
  });

  it("checks digit length, ordering, and repeat configuration", () => {
    validateEditorModel(digits);
    expect(() => validateEditorModel({ ...digits, ordered: false })).toThrow(/有序数字/);
    expect(() => validateEditorModel({ ...digits, length: 11 })).toThrow(/有序数字/);
  });
});

describe("definition validation", () => {
  it("accepts each selection mode and both mixed-tier policies", () => {
    const numbers = validDefinition();
    const second = structuredClone(numbers.prize_tiers[0]);
    second.code = "ADD_ON";
    second.exclusive = false;
    numbers.prize_tiers.push(second);
    numbers.mixed_tier_policy = "max_exclusive_plus_additive";
    expect(() => validateEditorDefinition(numbers)).not.toThrow();
    numbers.mixed_tier_policy = "max_all";
    expect(() => validateEditorDefinition(numbers)).not.toThrow();
    numbers.mixed_tier_policy = "";
    expect(() => validateEditorDefinition(numbers)).toThrow(/必须显式选择/);

    const excluded = validDefinition();
    excluded.selection = { mode: "exclude", regular_count: 0, special_count: 0, exclude_count: 2, attribute_groups: [], feature_choices: {} };
    excluded.prize_tiers[0].condition = { op: "equals", field: "excluded_match", target: "all", value: 1 };
    expect(() => validateEditorDefinition(excluded)).not.toThrow();

    const attributes = validDefinition();
    attributes.number_attributes = { Zone: { North: [1, 2] } };
    attributes.selection = { mode: "attributes", regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: ["Zone"], feature_choices: {} };
    attributes.prize_tiers[0].condition = { op: "equals", field: "attribute_match", target: "regular", attribute_group: "Zone", attribute_value: "$selection", value: 1 };
    expect(() => validateEditorDefinition(attributes)).not.toThrow();

    const features = validDefinition();
    features.selection = { mode: "features", regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: [], feature_choices: { Sum: [4, 7] } };
    features.prize_tiers[0].condition = { op: "selected", field: "draw_sum", target: "regular", selection_key: "Sum" };
    expect(() => validateEditorDefinition(features)).not.toThrow();
  });

  it("validates not, nested logic, selected leaves, and per-tier depth/node bounds", () => {
    const definition = validDefinition();
    definition.prize_tiers[0].condition = {
      op: "not",
      children: [{ op: "any", children: [
        { op: "equals", field: "regular_match", value: 0 },
        { op: "between", field: "regular_match", min: 2, max: 5 },
      ] }],
    };
    expect(() => validateEditorDefinition(definition)).not.toThrow();

    const tooDeep = validDefinition();
    const root: RuleDefinition["prize_tiers"][number]["condition"] = { op: "not", children: [] };
    tooDeep.prize_tiers[0].condition = root;
    let node = root;
    for (let i = 1; i < 8; i++) {
      const child: RuleDefinition["prize_tiers"][number]["condition"] = { op: "not", children: [] };
      node.children = [child];
      node = child;
    }
    node.children = [{ op: "equals", field: "regular_match", value: 1 }];
    expect(() => validateEditorDefinition(tooDeep)).toThrow(/深度/);

    const tooManyNodes = validDefinition();
    tooManyNodes.prize_tiers[0].condition = {
      op: "all",
      children: Array.from({ length: 32 }, () => ({ op: "all" as const, children: Array.from({ length: 4 }, () => ({ op: "equals" as const, field: "regular_match" as const, value: 1 })) })),
    };
    expect(() => validateEditorDefinition(tooManyNodes)).toThrow(/128 个节点/);

    const perTier = validDefinition();
    const another = structuredClone(perTier.prize_tiers[0]);
    another.code = "SECOND";
    perTier.prize_tiers[0].condition = tooManyNodes.prize_tiers[0].condition;
    expect(() => validateEditorDefinition(perTier)).toThrow(/128 个节点/);
    perTier.prize_tiers[0].condition = { op: "equals", field: "regular_match", value: 1 };
    perTier.prize_tiers.push({ ...another, condition: { op: "equals", field: "regular_match", value: 1 } });
    expect(() => validateEditorDefinition(perTier)).not.toThrow();
  });

  it("rejects unknown condition parameters, duplicates, and invalid targets", () => {
    const unknown = validDefinition();
    (unknown.prize_tiers[0].condition as unknown as Record<string, unknown>).script = "x";
    expect(() => validateEditorDefinition(unknown)).toThrow(/未知字段/);

    const duplicate = validDefinition();
    duplicate.prize_tiers.push(structuredClone(duplicate.prize_tiers[0]));
    expect(() => validateEditorDefinition(duplicate)).toThrow(/重复/);

    const target = validDefinition();
    target.prize_tiers[0].condition = { op: "equals", field: "draw_sum", target: "digits", value: 1 };
    expect(() => validateEditorDefinition(target)).toThrow(/digits 开奖范围/);
  });

  it("enforces bounded tiers, attribute groups, labels, values, and feature choices", () => {
    const tiers = validDefinition();
    tiers.prize_tiers = Array.from({ length: 33 }, (_, i) => ({
      ...structuredClone(tiers.prize_tiers[0]), code: `TIER_${i}`,
    }));
    expect(() => validateEditorDefinition(tiers)).toThrow(/奖级数量/);

    const groups = validDefinition();
    groups.number_attributes = Object.fromEntries(Array.from({ length: 17 }, (_, i) => [`G${i}`, { A: [1] }]));
    expect(() => validateEditorDefinition(groups)).toThrow(/最多 16 个/);

    const labels = validDefinition();
    labels.number_attributes = { Group: Object.fromEntries(Array.from({ length: 33 }, (_, i) => [`L${i}`, [1]])) };
    expect(() => validateEditorDefinition(labels)).toThrow(/1–32 个标签/);

    const members = validDefinition();
    members.number_attributes = { Group: { Label: Array.from({ length: 10_001 }, (_, i) => i) } };
    expect(() => validateEditorDefinition(members)).toThrow(/1–10000/);

    const features = validDefinition();
    features.selection = {
      mode: "features", regular_count: 0, special_count: 0, exclude_count: 0,
      attribute_groups: [], feature_choices: Object.fromEntries(Array.from({ length: 17 }, (_, i) => [`F${i}`, [1]])),
    };
    expect(() => validateEditorDefinition(features)).toThrow(/1–16 个特征/);
  });

  it("accepts exact large point strings and rejects float odds and int64 overflow", () => {
    const definition = validDefinition();
    definition.unit_points = "9223372036854775807";
    definition.limits.max_multiplier = "9223372036854775807";
    definition.cap_points = "9223372036854775807";
    expect(() => validateEditorDefinition(definition)).not.toThrow();
    definition.limits.max_multiplier = "9223372036854775808";
    expect(() => validateEditorDefinition(definition)).toThrow(/int64/);
    definition.limits.max_multiplier = "1000";
    definition.prize_tiers[0].odds = "2.5e3";
    expect(() => validateEditorDefinition(definition)).toThrow(/赔率/);
    expect(() => validateEditorPoints("01")).toThrow(/规范/);
    expect(() => validateEditorPoints("0")).toThrow(/大于零/);
    expect(() => validateEditorPoints("0", true)).not.toThrow();
    expect(() => validateEditorPoints("9223372036854775808", true)).toThrow(/int64/);
  });
});

describe("editor helpers", () => {
  it("parses bounded canonical numbers without sharing input state", () => {
    const parsed = parseEditorNumbers("0, 12\n20000000", 3);
    expect(parsed).toEqual([0, 12, 20_000_000]);
    expect(parsed).not.toBe([0, 12, 20_000_000]);
    expect(parseEditorNumbers(" ")).toEqual([]);
    expect(() => parseEditorNumbers("01")).toThrow(/规范/);
    expect(() => parseEditorNumbers("20000001")).toThrow(/0–20000000/);
    expect(() => parseEditorNumbers("1 2 3", 2)).toThrow(/不能超过 2/);
    expect(() => parseEditorNumbers("3,3")).toThrow(/重复/);
  });

  it("hydrates Go-omitted pool zeroes and null collections while preserving DSL values", () => {
    const definition = validDefinition();
    definition.model.regular_pool.min = undefined as unknown as number;
    definition.model.regular_pool.max = undefined as unknown as number;
    definition.model.regular_pool.values = null as unknown as number[];
    definition.selection.attribute_groups = null as unknown as string[];
    definition.selection.feature_choices = null as unknown as Record<string, number[]>;
    definition.number_attributes = null as unknown as RuleDefinition["number_attributes"];
    definition.prize_tiers[0].condition = {
      op: "in", field: "regular_match", values: [1, 3], children: [],
    };
    definition.prize_tiers[0].odds = "12.340001";
    definition.mixed_tier_policy = "max_exclusive_plus_additive";
    const originalCondition = structuredClone(definition.prize_tiers[0].condition);
    const hydrated = normalizeEditorDefinition(definition);
    expect(hydrated.model.regular_pool).toEqual({ min: 0, max: 0, values: [], allow_repeat: false });
    expect(hydrated.selection.attribute_groups).toEqual([]);
    expect(hydrated.selection.feature_choices).toEqual({});
    expect(hydrated.number_attributes).toEqual({});
    expect(hydrated.prize_tiers[0].condition).toEqual(originalCondition);
    expect(hydrated.prize_tiers[0].odds).toBe("12.340001");
    expect(hydrated.mixed_tier_policy).toBe("max_exclusive_plus_additive");
    expect(definition.selection.attribute_groups).toBeNull();
  });

  it("reports UTF-8 JSON bytes", () => {
    const definition = validDefinition();
    expect(editorDefinitionBytes(definition)).toBe(new TextEncoder().encode(JSON.stringify(definition)).length);
  });
});
