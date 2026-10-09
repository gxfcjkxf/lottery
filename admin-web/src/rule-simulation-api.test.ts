import { describe, expect, it, vi } from "vitest";
import {
  buildRuleSimulationRequest,
  canSimulateRules,
  createRuleSimulationApi,
  createRuleSimulationKeyTracker,
  DEFAULT_RULE_SIMULATION_FORM,
  effectiveRuleBrandPermissions,
  SAFE_CONDITION_FIELDS,
  type RuleSimulation,
  type RuleSimulationForm,
  type RuleTemplate,
} from "./rule-simulation-api";

const sampleResult: RuleSimulation = {
  won: true,
  normalized: {
    regular: [],
    special: [7],
    digits: [],
    exclude: [],
    attributes: {},
    features: {},
  },
  combination_count: 1,
  multiplier: "1",
  bet_points: "1",
  prize_points: "35",
  lines: [],
  warnings: [],
};
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });
const fail = (status: number, envelope = true) =>
  new Response(
    JSON.stringify(
      envelope
        ? {
            success: false,
            error: { code: `error_${status}`, message: `status ${status}` },
          }
        : { success: true },
    ),
    { status },
  );

describe("rule simulation API", () => {
  it("posts same-origin JSON with brand and idempotency headers", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(sampleResult));
    const api = createRuleSimulationApi(fetcher);
    const body = buildRuleSimulationRequest(
      "special",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    const output = await api.simulate("brand-1", body, "simulation-key");
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/rule-simulations");
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("same-origin");
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Brand-ID")).toBe("brand-1");
    expect(headers.get("Idempotency-Key")).toBe("simulation-key");
    expect(headers.get("Authorization")).toBeNull();
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(output.prize_points).toBe("35");
  });

  it("returns API errors, including success envelopes with no data", async () => {
    for (const status of [401, 403, 409]) {
      const api = createRuleSimulationApi(
        vi.fn<typeof fetch>().mockResolvedValue(fail(status)),
      );
      await expect(
        api.simulate(
          "b",
          buildRuleSimulationRequest("special", DEFAULT_RULE_SIMULATION_FORM),
          "k",
        ),
      ).rejects.toMatchObject({ status, code: `error_${status}` });
    }
    const malformed = createRuleSimulationApi(
      vi.fn<typeof fetch>().mockResolvedValue(fail(200, false)),
    );
    await expect(
      malformed.simulate(
        "b",
        buildRuleSimulationRequest("special", DEFAULT_RULE_SIMULATION_FORM),
        "k",
      ),
    ).rejects.toMatchObject({ status: 200 });
  });

  it("grants only mapped staff simulation permission", () => {
    const actor = {
      super_admin: false,
      brand_ids: ["a"],
      permissions_by_brand: { a: ["rule.simulate.brand"] },
    };
    expect(canSimulateRules(actor, "a")).toBe(true);
    expect(canSimulateRules(actor, "b")).toBe(false);
    expect(effectiveRuleBrandPermissions(actor, "b").size).toBe(0);
    expect(
      canSimulateRules(
        {
          super_admin: true,
          brand_ids: [],
        },
        "b",
      ),
    ).toBe(false);
    expect(
      canSimulateRules(
        {
          super_admin: true,
          brand_ids: ["b"],
          permissions_by_brand: { b: ["rule.simulate.brand"] },
        },
        "b",
      ),
    ).toBe(false);
  });

  it.each<RuleTemplate>([
    "special",
    "digits",
    "features",
    "exclude",
    "attributes",
    "m-select-n",
  ])(
    "builds complete schema-v1 %s payload with explicit rounding scope",
    (template) => {
      const form: RuleSimulationForm = structuredClone(
        DEFAULT_RULE_SIMULATION_FORM,
      );
      const body = buildRuleSimulationRequest(template, form);
      expect(body.definition.schema_version).toBe(1);
      expect(body.definition.rounding).toBe("half_up");
      expect(body.definition.rounding_scope).toBe("order");
      expect(body.definition.limits).toEqual({
        max_combinations: 10000,
        max_multiplier: "1000",
        max_bet_points: null,
      });
      expect(body.definition.number_attributes).toBeDefined();
      expect(body.definition.selection.feature_choices).toBeDefined();
      expect(body.multiplier).toBe("1");
      if (body.definition.model.model !== "DIGITS_0_9") {
        expect(body.draw.regular).toHaveLength(
          body.definition.model.regular_count,
        );
        expect(body.draw.special).toHaveLength(
          body.definition.model.special_count,
        );
      } else {
        expect(body.draw.digits).toHaveLength(body.definition.model.length);
      }
    },
  );

  it("builds the documented template shapes and whitelisted feature AST", () => {
    const special = buildRuleSimulationRequest(
      "special",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    expect(special.definition.model).toMatchObject({
      model: "X_PLUS_Y",
      regular_count: 6,
      special_count: 1,
    });
    expect(special.selection).toMatchObject({
      regular: [],
      special: [7, 19, 31, 43],
    });
    expect(special.draw).toMatchObject({
      regular: [1, 2, 3, 4, 5, 6],
      special: [7],
    });
    expect(special.definition.prize_tiers[0].condition).toMatchObject({
      op: "all",
      children: [{ op: "equals", field: "special_match", value: 1 }],
    });
    expect(
      special.definition.prize_tiers[0].condition.children?.[0],
    ).not.toHaveProperty("target");

    const digits = buildRuleSimulationRequest(
      "digits",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    expect(digits.selection.digits).toEqual([[3], [1], [0]]);
    expect(digits.definition.model).toMatchObject({
      model: "DIGITS_0_9",
      length: 3,
      ordered: true,
      regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
      special_pool: { min: 0, max: 0, values: [], allow_repeat: false },
    });
    expect(digits.definition.prize_tiers[0].condition.children?.[0]).toEqual({
      op: "equals",
      field: "position_match",
      value: 3,
    });

    const features = buildRuleSimulationRequest(
      "features",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    expect(features.definition.selection.feature_choices).toEqual({
      three_kind: [0, 1],
      same_ends: [0, 1],
      odd_count: [0, 1, 2, 3],
      sum: Array.from({ length: 28 }, (_, index) => index),
    });
    expect(features.selection.features).toEqual({
      three_kind: [0],
      same_ends: [0],
      odd_count: [2],
      sum: [4],
    });
    expect(features.definition.prize_tiers[0].condition.children).toEqual([
      {
        op: "selected",
        field: "draw_all_same",
        target: "digits",
        selection_key: "three_kind",
      },
      {
        op: "selected",
        field: "draw_first_last_same",
        target: "digits",
        selection_key: "same_ends",
      },
      {
        op: "selected",
        field: "draw_odd_count",
        target: "digits",
        selection_key: "odd_count",
      },
      {
        op: "selected",
        field: "draw_sum",
        target: "digits",
        selection_key: "sum",
      },
    ]);
    expect(
      features.definition.prize_tiers[0].condition.children?.every(
        (child) => child.op === "selected",
      ),
    ).toBe(true);

    const excluded = buildRuleSimulationRequest(
      "exclude",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    expect(excluded.selection.exclude).toEqual([11, 22, 33]);
    expect(
      excluded.definition.prize_tiers[0].condition.children?.[0],
    ).toMatchObject({ field: "excluded_match", value: 0 });

    const attributes = buildRuleSimulationRequest(
      "attributes",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    expect(attributes.definition.number_attributes.color).toEqual({
      red: [1, 3, 5, 7, 9],
      blue: [2, 4, 6, 8],
    });
    expect(attributes.selection.attributes).toEqual({ color: ["red", "blue"] });

    const mSelect = buildRuleSimulationRequest(
      "m-select-n",
      DEFAULT_RULE_SIMULATION_FORM,
    );
    expect(mSelect.definition.model).toMatchObject({
      model: "M_SELECT_N",
      pool_size: 49,
      total_count: 7,
    });
    expect(mSelect.selection.regular).toEqual([1, 2, 3, 4, 5, 6]);
    expect(mSelect.definition.prize_tiers[0].condition.children).toEqual([
      { op: "equals", field: "regular_match", value: 6 },
      { op: "equals", field: "special_match", value: 1 },
    ]);
  });

  it("keeps points and odds exact strings and bounds decimal odds/multiplier", () => {
    const form = {
      ...structuredClone(DEFAULT_RULE_SIMULATION_FORM),
      unitPoints: "9007199254740993123456789",
      odds: "0.000001",
      multiplier: "1000",
      capPoints: "90071992547409931234567890",
      maxBetPoints: "90071992547409931234567890",
      roundingScope: "tier" as const,
    };
    const body = buildRuleSimulationRequest("special", form);
    expect(body.definition.unit_points).toBe(form.unitPoints);
    expect(body.definition.prize_tiers[0].odds).toBe("0.000001");
    expect(body.definition.cap_points).toBe(form.capPoints);
    expect(body.definition.rounding_scope).toBe("tier");
    expect(body.multiplier).toBe("1000");
    expect(() =>
      buildRuleSimulationRequest("special", { ...form, odds: "1.1234567" }),
    ).toThrow();
    expect(() =>
      buildRuleSimulationRequest("special", {
        ...form,
        multiplier: "1000.000001",
      }),
    ).toThrow();
    expect(() =>
      buildRuleSimulationRequest("special", { ...form, multiplier: "1.5" }),
    ).toThrow();
  });

  it("limits condition AST to eight safe leaves and rejects unknown feature codes", () => {
    const extras = Array.from({ length: 7 }, () => ({
      field: "draw_sum" as const,
      operator: "equals" as const,
      value: "4",
      max: "9",
      target: "all" as const,
      position: "0",
    }));
    const form = {
      ...structuredClone(DEFAULT_RULE_SIMULATION_FORM),
      extraConditions: extras,
    };
    expect(
      buildRuleSimulationRequest("special", form).definition.prize_tiers[0]
        .condition.children,
    ).toHaveLength(8);
    expect(() =>
      buildRuleSimulationRequest("special", {
        ...form,
        extraConditions: [...extras, extras[0]],
      }),
    ).toThrow("1 至 8");
    expect(() =>
      buildRuleSimulationRequest("features", {
        ...structuredClone(DEFAULT_RULE_SIMULATION_FORM),
        featureValues: { arbitrary_code: "1" },
      }),
    ).toThrow("不合法");
  });

  it("reuses identical retry keys but rotates on body or brand scope changes", () => {
    const keyFor = createRuleSimulationKeyTracker();
    const first = {
      brand_id: "b1",
      definition: { schema_version: 1 },
      multiplier: "1",
    };
    const key = keyFor(first);
    expect(keyFor({ ...first })).toBe(key);
    expect(keyFor({ ...first, brand_id: "b2" })).not.toBe(key);
    expect(keyFor({ ...first, multiplier: "2" })).not.toBe(key);
  });

  it("uses only backend condition field whitelist names", () => {
    expect(SAFE_CONDITION_FIELDS).toEqual([
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
    ]);
    const body = buildRuleSimulationRequest("digits", {
      ...structuredClone(DEFAULT_RULE_SIMULATION_FORM),
      extraConditions: [
        {
          field: "position_match",
          operator: "equals",
          value: "1",
          max: "9",
          target: "digits",
          position: "2",
        },
      ],
    });
    expect(
      body.definition.prize_tiers[0].condition.children?.[1],
    ).toMatchObject({
      field: "position_match",
      value: 1,
    });
    expect(
      body.definition.prize_tiers[0].condition.children?.[1],
    ).not.toHaveProperty("position");
    expect(
      body.definition.prize_tiers[0].condition.children?.[1],
    ).not.toHaveProperty("target");
  });
});
