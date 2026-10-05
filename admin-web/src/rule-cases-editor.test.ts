import { describe, expect, it } from "vitest";
import type { RuleDefinition, RuleModelName } from "./rule-simulation-api";
import type { RuleValidationCase } from "./rule-versions-api";
import {
  defaultRuleCase,
  normalizeRuleCase,
  validateRuleCase,
  validateRuleCases,
} from "./rule-cases-editor";

type SelectionMode = RuleDefinition["selection"]["mode"];

function definition(
  modelName: RuleModelName,
  mode: SelectionMode,
  repeats = false,
): RuleDefinition {
  const model: RuleDefinition["model"] =
    modelName === "DIGITS_0_9"
      ? {
          model: modelName,
          regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
          special_pool: { min: 0, max: 0, values: [], allow_repeat: false },
          regular_count: 0,
          special_count: 0,
          pool_size: 0,
          total_count: 0,
          length: 3,
          allow_repeat: repeats,
          ordered: true,
        }
      : modelName === "M_SELECT_N"
        ? {
            model: modelName,
            regular_pool: {
              min: 0,
              max: 0,
              values: [1, 2, 3, 4, 5, 6],
              allow_repeat: false,
            },
            special_pool: {
              min: 0,
              max: 0,
              values: [1, 2, 3, 4, 5, 6],
              allow_repeat: false,
            },
            regular_count: 2,
            special_count: 1,
            pool_size: 6,
            total_count: 3,
            length: 0,
            allow_repeat: false,
            ordered: false,
          }
        : {
            model: modelName,
            regular_pool: { min: 1, max: 6, values: [], allow_repeat: repeats },
            special_pool: { min: 11, max: 16, values: [], allow_repeat: false },
            regular_count: 2,
            special_count: 1,
            pool_size: 0,
            total_count: 0,
            length: 0,
            allow_repeat: false,
            ordered: false,
          };
  const isNumbers = mode === "numbers";
  return {
    schema_version: 1,
    model,
    selection: {
      mode,
      regular_count: isNumbers && modelName !== "DIGITS_0_9" ? 2 : 0,
      special_count: isNumbers && modelName !== "DIGITS_0_9" ? 1 : 0,
      exclude_count: mode === "exclude" ? 2 : 0,
      attribute_groups: mode === "attributes" ? ["band"] : [],
      feature_choices: mode === "features" ? { sum: [1, 2, 3] } : {},
    },
    number_attributes: {
      band: { warm: modelName === "DIGITS_0_9" ? [0, 1] : [1, 2] },
    },
    unit_points: "1",
    prize_tiers: [],
    mixed_tier_policy: "",
    cap_points: null,
    rounding: "half_up",
    rounding_scope: "order",
    limits: {
      max_combinations: 100,
      max_multiplier: "1000",
      max_bet_points: null,
    },
  };
}

function validCase(def: RuleDefinition): RuleValidationCase {
  const model = def.model;
  const selection: RuleValidationCase["selection"] = {
    regular: [],
    special: [],
    digits: [],
    exclude: [],
    attributes: {},
    features: {},
  };
  const draw: RuleValidationCase["draw"] = {
    regular: [],
    special: [],
    digits: [],
  };
  if (model.model === "DIGITS_0_9") {
    draw.digits = [0, 1, 2];
    if (def.selection.mode === "numbers")
      selection.digits = [
        [0, 1],
        [1, 2],
        [2, 3],
      ];
  } else if (model.model === "M_SELECT_N") {
    draw.regular = [1, 2];
    draw.special = [3];
    if (def.selection.mode === "numbers") {
      selection.regular = [1, 2, 3];
      selection.special = [2, 3];
    }
  } else {
    draw.regular = [1, 2];
    draw.special = [11];
    if (def.selection.mode === "numbers") {
      selection.regular = [1, 2, 3];
      selection.special = [11];
    }
  }
  if (def.selection.mode === "exclude") selection.exclude = [1, 2];
  if (def.selection.mode === "attributes")
    selection.attributes = { band: ["warm"] };
  if (def.selection.mode === "features") selection.features = { sum: [1, 3] };
  return {
    name: "case",
    selection,
    draw,
    multiplier: "1",
    expected_bet_points: "0",
    expected_prize_points: "0",
    expected_won: false,
  };
}

describe("rule case editor validation", () => {
  it("accepts all DSL models and selection modes with their own input shapes", () => {
    for (const model of ["X_PLUS_Y", "M_SELECT_N", "DIGITS_0_9"] as const) {
      for (const mode of [
        "numbers",
        "exclude",
        "attributes",
        "features",
      ] as const) {
        const def = definition(model, mode);
        expect(
          () => validateRuleCase(def, validCase(def)),
          `${model}/${mode}`,
        ).not.toThrow();
      }
    }
  });

  it("uses pool repeat flags for X_PLUS_Y and model repeat only for digit draws", () => {
    const repeatedPool = definition("X_PLUS_Y", "numbers", true);
    const repeatedCase = validCase(repeatedPool);
    repeatedCase.draw.regular = [1, 1];
    repeatedCase.selection.regular = [1];
    expect(() => validateRuleCase(repeatedPool, repeatedCase)).not.toThrow();

    const uniquePool = definition("X_PLUS_Y", "numbers", false);
    const invalidDraw = validCase(uniquePool);
    invalidDraw.draw.regular = [1, 1];
    expect(() => validateRuleCase(uniquePool, invalidDraw)).toThrow(
      /duplicate/i,
    );

    const repeatDigits = definition("DIGITS_0_9", "numbers", true);
    const digitsCase = validCase(repeatDigits);
    digitsCase.draw.digits = [0, 0, 0];
    expect(() => validateRuleCase(repeatDigits, digitsCase)).not.toThrow();
    const uniqueDigits = definition("DIGITS_0_9", "numbers", false);
    digitsCase.draw.digits = [0, 0, 1];
    expect(() => validateRuleCase(uniqueDigits, digitsCase)).toThrow(
      /duplicate/i,
    );
  });

  it("deduplicates candidate spelling for feasibility like the backend", () => {
    const d = definition("X_PLUS_Y", "numbers");
    const c = validCase(d);
    c.selection.regular = [1, 1, 2];
    expect(() => validateRuleCase(d, c)).not.toThrow();
    c.selection.regular = [1, 1];
    expect(() => validateRuleCase(d, c)).toThrow();
  });

  it("enforces M-select distinct feasible selections and separate draws", () => {
    const def = definition("M_SELECT_N", "numbers");
    const item = validCase(def);
    item.selection.regular = [1, 2];
    item.selection.special = [1];
    expect(() => validateRuleCase(def, item)).toThrow(
      /distinct regular and special/i,
    );
    item.selection.regular = [1, 2, 3];
    item.selection.special = [2, 3];
    item.draw.special = [2];
    expect(() => validateRuleCase(def, item)).toThrow(
      /draw values must be distinct/i,
    );
  });

  it("requires exact exclude counts, allowed attribute labels and feature choices", () => {
    const exclude = definition("X_PLUS_Y", "exclude");
    const excluded = validCase(exclude);
    excluded.selection.exclude = [1, 11, 2];
    expect(() => validateRuleCase(exclude, excluded)).toThrow(/exactly/i);

    const attributes = definition("X_PLUS_Y", "attributes");
    const attributed = validCase(attributes);
    attributed.selection.attributes.band = ["unknown"];
    expect(() => validateRuleCase(attributes, attributed)).toThrow(
      /unavailable value/i,
    );

    const features = definition("DIGITS_0_9", "features");
    const featured = validCase(features);
    featured.selection.features.sum = [9];
    expect(() => validateRuleCase(features, featured)).toThrow(
      /unavailable value/i,
    );
  });

  it("preserves arbitrary precision canonical point strings and bounds multiplier", () => {
    const def = definition("X_PLUS_Y", "numbers");
    const item = validCase(def);
    item.expected_bet_points = "9007199254740993";
    item.expected_prize_points = "9223372036854775807";
    item.multiplier = "1000";
    expect(() => validateRuleCase(def, item)).not.toThrow();
    for (const bad of ["01", "+1", "1.0", "-1", "9223372036854775808"]) {
      item.expected_bet_points = bad;
      expect(() => validateRuleCase(def, item), bad).toThrow();
    }
    item.expected_bet_points = "0";
    item.multiplier = "1001";
    expect(() => validateRuleCase(def, item)).toThrow(/multiplier/i);
  });

  it("checks UTF-8 byte limits and rejects NUL case names", () => {
    const def = definition("X_PLUS_Y", "numbers");
    const item = validCase(def);
    item.name = "é".repeat(60);
    expect(() => validateRuleCase(def, item)).not.toThrow();
    item.name += "é";
    expect(() => validateRuleCase(def, item)).toThrow(/120 bytes/i);
    item.name = "valid\0name";
    expect(() => validateRuleCase(def, item)).toThrow(/NUL-free/i);
  });

  it("enforces suite size and unique trimmed names up to 32 cases", () => {
    const def = definition("DIGITS_0_9", "numbers");
    const cases = Array.from({ length: 32 }, (_, index) => ({
      ...validCase(def),
      name: `case-${index + 1}`,
    }));
    expect(() => validateRuleCases(def, cases)).not.toThrow();
    expect(() =>
      validateRuleCases(def, [
        ...cases,
        { ...validCase(def), name: "case-33" },
      ]),
    ).toThrow(/1 and 32/i);
    cases[31]!.name = " case-1 ";
    expect(() => validateRuleCases(def, cases)).toThrow(/unique/i);
  });

  it("creates a valid minimal default without implying a pass", () => {
    for (const model of ["X_PLUS_Y", "M_SELECT_N", "DIGITS_0_9"] as const) {
      const def = definition(model, "numbers");
      const item = defaultRuleCase(def);
      expect(item.expected_bet_points).toBe("0");
      expect(item.expected_prize_points).toBe("0");
      expect(item.expected_won).toBe(false);
      expect(() => validateRuleCase(def, item)).not.toThrow();
    }
  });

  it("normalizes Go nil collections without coercing scalar values", () => {
    const source = {
      ...validCase(definition("DIGITS_0_9", "numbers")),
      multiplier: "9007199254740993",
      selection: {
        regular: null,
        special: null,
        digits: [null, [0], null],
        exclude: null,
        attributes: null,
        features: null,
      },
      draw: { regular: null, special: null, digits: [0, 1, 2] },
    } as unknown as RuleValidationCase;
    const normalized = normalizeRuleCase(source);
    expect(normalized.selection).toEqual({
      regular: [],
      special: [],
      digits: [[], [0], []],
      exclude: [],
      attributes: {},
      features: {},
    });
    expect(normalized.multiplier).toBe("9007199254740993");
    expect(source.selection.regular).toBeNull();
  });

  it("leaves required attribute groups invalid after nil collections are normalized", () => {
    const def = definition("X_PLUS_Y", "attributes");
    const item = validCase(def);
    item.selection.attributes =
      null as unknown as RuleValidationCase["selection"]["attributes"];
    expect(() => validateRuleCase(def, item)).toThrow(
      /every configured attribute group/i,
    );
  });
});
