import { afterEach, describe, expect, it, vi } from "vitest";
import type { AdminAccount } from "./admin-api";
import {
  buildRuleSimulationRequest,
  type RuleDefinition,
  type RuleTemplate,
} from "./rule-simulation-api";
import {
  buildRuleValidationCase,
  canCloneRuleVersion,
  canReviewRuleVersion,
  createRuleVersionKeyTracker,
  createRuleVersionRequestGuard,
  createRuleVersionsApi,
  defaultRuleVersionForm,
  restoreRuleVersionTemplate,
  ruleDefinitionSignature,
  ruleTemplateModel,
  ruleVersionDefinitionLocked,
  ruleVersionPermissions,
  type GameRecord,
  type PlayRecord,
  type RuleVersion,
} from "./rule-versions-api";

const brand = "brand-a";
const form = defaultRuleVersionForm("special");
const definition = buildRuleSimulationRequest("special", form).definition;
const inputCase = buildRuleValidationCase("special", form, {
  name: "特别号中奖",
  bet: "4",
  prize: "35",
  won: true,
});
const version: RuleVersion = {
  id: "version-1",
  brand_id: brand,
  game_id: "game-1",
  play_id: "play-1",
  version_no: 2,
  version: 7,
  definition,
  definition_hash: "definition-hash",
  status: "draft",
  effect_mode: "next_period",
  created_by: "creator",
  reviewed_by: "",
  review_comment: "",
  created_at: "2026-10-06T00:00:00Z",
  updated_at: "2026-10-06T00:00:00Z",
  audit_log_id: "audit-1",
  validation: {
    passed: false,
    definition_hash: "definition-hash",
    cases: [
      {
        name: inputCase.name,
        input: inputCase,
        expected_bet_points: "4",
        actual_bet_points: "4",
        expected_prize_points: "35",
        actual_prize_points: "0",
        expected_won: true,
        actual_won: false,
        matched: false,
        simulation: {
          won: false,
          normalized: inputCase.selection,
          combination_count: 4,
          multiplier: "1",
          bet_points: "4",
          prize_points: "0",
          raw_prize_points: "0/1",
          capped_prize_points: "0/1",
          lines: [],
          warnings: [],
        },
      },
    ],
    warnings: ["规则审批会影响正式投注"],
    findings: [
      { code: "CASE_MISMATCH", message: "中奖预期不符合", blocking: true },
    ],
  },
};
const game: GameRecord = {
  id: "game-1",
  brand_id: brand,
  code: "marksix",
  name: "六合彩",
  model: definition.model,
  timezone: "Asia/Manila",
  version: 1,
  status: "active",
  started_sequence: 0,
};
const play: PlayRecord = {
  id: "play-1",
  brand_id: brand,
  game_id: game.id,
  code: "special",
  name: "特别号",
  status: "active",
  active_version_id: "",
  version: 1,
};
const actor: AdminAccount = {
  id: "reviewer",
  super_admin: false,
  brand_ids: [brand],
  permissions: [],
  platform_permissions: [],
  permissions_by_brand: {
    [brand]: [
      "game.view.brand",
      "game.write.brand",
      "rule.view.brand",
      "rule.write.brand",
      "rule.validate.brand",
      "rule.submit.brand",
      "rule.review.brand",
    ],
  },
};
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });
afterEach(() => vi.unstubAllGlobals());

describe("rule version client", () => {
  it("GETs real catalog/history envelopes with cookies, explicit brand and encoded targets", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ games: [game] }))
      .mockResolvedValueOnce(ok({ plays: [play] }))
      .mockResolvedValueOnce(ok({ versions: [version] }));
    const api = createRuleVersionsApi(fetcher);
    expect((await api.getGames(brand)).games[0].model).toEqual(
      definition.model,
    );
    expect((await api.getPlays(brand, "game/one")).plays).toEqual([play]);
    expect((await api.getRuleVersions(brand, "play/one")).versions).toEqual([
      version,
    ]);
    expect(fetcher.mock.calls.map(([path]) => path)).toEqual([
      "/api/v1/admin/games",
      "/api/v1/admin/games/game%2Fone/plays",
      "/api/v1/admin/plays/play%2Fone/rule-versions",
    ]);
    for (const [, init] of fetcher.mock.calls) {
      const headers = new Headers(init?.headers);
      expect(init?.method).toBe("GET");
      expect(init?.credentials).toBe("same-origin");
      expect(headers.get("X-Brand-ID")).toBe(brand);
      expect(headers.get("Idempotency-Key")).toBeNull();
      expect(headers.get("Content-Type")).toBeNull();
      expect(headers.get("Authorization")).toBeNull();
      expect(init?.body).toBeUndefined();
    }
  });

  it("POSTs a full game Model, creates play/draft, and PUTs CAS definition without numeric coercion", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(game))
      .mockResolvedValueOnce(ok(play))
      .mockResolvedValueOnce(ok(version))
      .mockResolvedValueOnce(ok(version));
    const api = createRuleVersionsApi(fetcher);
    const gameBody = {
      code: game.code,
      name: game.name,
      model: definition.model,
      timezone: game.timezone,
      reason: "创建彩种",
    };
    const playBody = { code: play.code, name: play.name, reason: "创建玩法" };
    const draftBody = {
      play_id: play.id,
      definition,
      effect_mode: "next_period" as const,
      reason: "首版草稿",
    };
    const updateBody = {
      version: 7,
      definition: { ...definition, unit_points: "9007199254740993" },
      effect_mode: "immediate" as const,
      reason: "修改草稿",
    };
    await api.createGame(brand, gameBody, "game-key");
    await api.createPlay(brand, game.id, playBody, "play-key");
    await api.createRuleVersion(brand, draftBody, "draft-key");
    await api.updateRuleVersion(brand, "version/one", updateBody, "update-key");
    const expected = [
      ["/api/v1/admin/games", "POST", gameBody, "game-key"],
      ["/api/v1/admin/games/game-1/plays", "POST", playBody, "play-key"],
      ["/api/v1/admin/rule-versions", "POST", draftBody, "draft-key"],
      [
        "/api/v1/admin/rule-versions/version%2Fone",
        "PUT",
        updateBody,
        "update-key",
      ],
    ];
    fetcher.mock.calls.forEach(([url, init], index) => {
      const [path, method, body, key] = expected[index];
      const headers = new Headers(init?.headers);
      expect(url).toBe(path);
      expect(init?.method).toBe(method);
      expect(init?.credentials).toBe("same-origin");
      expect(headers.get("Content-Type")).toBe("application/json");
      expect(headers.get("X-Brand-ID")).toBe(brand);
      expect(headers.get("Idempotency-Key")).toBe(key);
      expect(headers.get("Authorization")).toBeNull();
      expect(headers.get("Origin")).toBeNull(); // The browser supplies its own Origin.
      expect(JSON.parse(String(init?.body))).toEqual(body);
    });
  });

  it("sends the exact validation/review/clone bodies and preserves failed reports", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => ok(version));
    const api = createRuleVersionsApi(fetcher);
    const cas = { version: 7, reason: "运营核对" };
    const validate = { ...cas, cases: [inputCase] };
    expect(
      (
        await api.validateRuleVersion(
          brand,
          version.id,
          validate,
          "validate-key",
        )
      ).validation?.passed,
    ).toBe(false);
    await api.submitRuleVersion(brand, version.id, cas, "submit-key");
    await api.approveRuleVersion(
      brand,
      version.id,
      { ...cas, warnings_acknowledged: true },
      "approve-key",
    );
    await api.rejectRuleVersion(brand, version.id, cas, "reject-key");
    await api.cloneRuleVersion(
      brand,
      version.id,
      { effect_mode: "next_period", reason: "回滚草稿" },
      "clone-key",
    );
    const operations = [
      "validate",
      "submit-review",
      "approve",
      "reject",
      "clone",
    ];
    const bodies = [
      validate,
      cas,
      { ...cas, warnings_acknowledged: true },
      cas,
      { effect_mode: "next_period", reason: "回滚草稿" },
    ];
    fetcher.mock.calls.forEach(([url, init], index) => {
      expect(url).toBe(
        `/api/v1/admin/rule-versions/version-1/${operations[index]}`,
      );
      expect(init?.method).toBe("POST");
      expect(JSON.parse(String(init?.body))).toEqual(bodies[index]);
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
      expect(new Headers(init?.headers).get("Idempotency-Key")).toBeTruthy();
    });
  });

  it.each([
    [400, "RULE_INVALID"],
    [401, "SESSION_INVALID"],
    [403, "PERMISSION_DENIED"],
    [409, "RULE_VERSION_CONFLICT"],
    [409, "RULE_STATE_CONFLICT"],
    [409, "IDEMPOTENCY_CONFLICT"],
    [409, "RULE_VALIDATION_REQUIRED"],
  ])("preserves %s/%s without manufacturing success", async (status, code) => {
    const api = createRuleVersionsApi(
      vi.fn<typeof fetch>().mockResolvedValue(
        new Response(
          JSON.stringify({
            success: false,
            error: { code, message: "拒绝操作" },
          }),
          { status: Number(status) },
        ),
      ),
    );
    await expect(
      api.approveRuleVersion(
        brand,
        version.id,
        { version: 7, reason: "复核", warnings_acknowledged: true },
        "key",
      ),
    ).rejects.toMatchObject({ status, code, message: "拒绝操作" });
  });

  it("rejects non-JSON, null/missing data, failed 200 envelopes and missing brand; propagates network failure", async () => {
    const malformed = [
      "not json",
      "null",
      "[]",
      '{"success":true}',
      '{"success":true,"data":null}',
      '{"success":true,"data":false}',
      '{"success":true,"data":[]}',
      '{"success":false,"error":"拒绝"}',
    ];
    for (const raw of malformed) {
      const api = createRuleVersionsApi(
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(new Response(raw, { status: 200 })),
      );
      await expect(api.getGames(brand)).rejects.toMatchObject({ status: 200 });
    }
    const fetcher = vi
      .fn<typeof fetch>()
      .mockRejectedValue(new TypeError("Network failed"));
    const api = createRuleVersionsApi(fetcher);
    await expect(api.getGames("")).rejects.toMatchObject({ status: 0 });
    expect(fetcher).not.toHaveBeenCalled();
    await expect(api.getGames(brand)).rejects.toThrow("Network failed");
  });

  it("uses no token/storage and creates an idempotency key when one is omitted", async () => {
    const getItem = vi.fn(() => {
      throw new Error("storage must not be read");
    });
    const setItem = vi.fn(() => {
      throw new Error("storage must not be written");
    });
    vi.stubGlobal("localStorage", { getItem, setItem });
    vi.stubGlobal("sessionStorage", { getItem, setItem });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(version));
    await createRuleVersionsApi(fetcher).submitRuleVersion(brand, version.id, {
      version: 7,
      reason: "提交",
    });
    expect(
      new Headers(fetcher.mock.calls[0][1]?.headers).get("Idempotency-Key"),
    ).toMatch(/^[a-f0-9-]{36}$/);
    expect(getItem).not.toHaveBeenCalled();
    expect(setItem).not.toHaveBeenCalled();
  });
});

describe("rule version permission, retry and response scopes", () => {
  it("isolates brand maps, ignores flat grants, and keeps read/write stages separate", () => {
    expect(ruleVersionPermissions(actor, brand)).toEqual({
      gamesView: true,
      gamesWrite: true,
      rulesView: true,
      rulesWrite: true,
      validate: true,
      submit: true,
      review: true,
    });
    expect(Object.values(ruleVersionPermissions(actor, "brand-b"))).toEqual(
      Array(7).fill(false),
    );
    const writeOnly = {
      ...actor,
      permissions_by_brand: { [brand]: ["rule.write.brand"] },
    };
    expect(ruleVersionPermissions(writeOnly, brand)).toMatchObject({
      rulesWrite: true,
      rulesView: false,
      validate: false,
      submit: false,
      review: false,
    });
    const flat = {
      ...actor,
      permissions_by_brand: {},
      permissions: ["rule.view.platform"],
      platform_permissions: [],
    };
    expect(ruleVersionPermissions(flat, brand).rulesView).toBe(false);
    expect(
      ruleVersionPermissions(
        { ...flat, platform_permissions: undefined },
        brand,
      ).rulesView,
    ).toBe(false);
    expect(
      ruleVersionPermissions(
        {
          ...actor,
          permissions_by_brand: {},
          permissions: ["rule.view.brand"],
        },
        brand,
      ).rulesView,
    ).toBe(false);
    expect(
      ruleVersionPermissions(
        {
          ...actor,
          permissions_by_brand: undefined,
          permissions: ["rule.view.brand"],
        },
        brand,
        ).rulesView,
    ).toBe(false);
    expect(Object.values(ruleVersionPermissions(actor, ""))).toEqual(
      Array(7).fill(false),
    );
  });

  it("ignores platform grants and denies all super-admin rights", () => {
    const platform = {
      ...actor,
      permissions_by_brand: {},
      platform_permissions: [
        "game.view.platform",
        "rule.view.platform",
        "rule.write.platform",
        "rule.review.platform",
      ],
    };
    expect(ruleVersionPermissions(platform, brand)).toEqual({
      gamesView: false,
      rulesView: false,
      gamesWrite: false,
      rulesWrite: false,
      validate: false,
      submit: false,
      review: false,
    });
    expect(
      ruleVersionPermissions({ ...actor, super_admin: true }, brand),
    ).toEqual({
      gamesView: false,
      rulesView: false,
      gamesWrite: false,
      rulesWrite: false,
      validate: false,
      submit: false,
      review: false,
    });
  });

  it("allows creator review while requiring the pending version's brand permission", () => {
    const pending = { ...version, status: "pending_review" as const };
    expect(canReviewRuleVersion(actor, brand, pending)).toBe(true);
    expect(canReviewRuleVersion({ ...actor, id: pending.created_by }, brand, pending)).toBe(true);
    expect(canReviewRuleVersion(actor, "brand-b", pending)).toBe(false);
    expect(canReviewRuleVersion(actor, brand, version)).toBe(false);
    expect(
      canReviewRuleVersion(
        { ...actor, permissions_by_brand: {} },
        brand,
        pending,
      ),
    ).toBe(false);
    expect(
      canReviewRuleVersion({ ...actor, super_admin: true }, brand, pending),
    ).toBe(false);
  });

  it("uses target/brand/operation scope for retry keys, retaining a scope's key across other requests", () => {
    const keyFor = createRuleVersionKeyTracker();
    const body = { version: 7, reason: "批准", warnings_acknowledged: true };
    const first = keyFor(brand, "approve", "v1", body);
    expect(keyFor(brand, "approve", "v2", body)).not.toBe(first);
    expect(keyFor(brand, "reject", "v1", body)).not.toBe(first);
    expect(keyFor("brand-b", "approve", "v1", body)).not.toBe(first);
    expect(
      keyFor(brand, "approve", "v1", {
        reason: "批准",
        warnings_acknowledged: true,
        version: 7,
      }),
    ).toBe(first);
    expect(keyFor(brand, "approve", "v1", { ...body, version: 8 })).not.toBe(
      first,
    );
    expect(
      keyFor(brand, "approve", "v1", { ...body, reason: "更改原因" }),
    ).not.toBe(first);
  });

  it("discards superseded reads, cross-brand/input responses, and revoked permissions, even after returning to the old scope", () => {
    const guard = createRuleVersionRequestGuard();
    const scope = JSON.stringify([brand, actor.id, "play-1"]);
    const old = guard.capture("versions", scope);
    expect(guard.isCurrent(old, scope, true)).toBe(true);
    const latest = guard.capture("versions", scope);
    expect(guard.isCurrent(old, scope, true)).toBe(false);
    expect(guard.isCurrent(latest, scope, false)).toBe(false);
    expect(
      guard.isCurrent(
        latest,
        JSON.stringify(["brand-b", actor.id, "play-1"]),
        true,
      ),
    ).toBe(false);
    const write = guard.capture("write", "inputs-1");
    expect(guard.isCurrent(write, "inputs-2", true)).toBe(false);
    guard.invalidate();
    expect(guard.isCurrent(latest, scope, true)).toBe(false);
    expect(guard.isCurrent(write, "inputs-1", true)).toBe(false);
    expect(
      Object.values(
        ruleVersionPermissions(
          { ...actor, permissions_by_brand: {}, platform_permissions: [] },
          brand,
        ),
      ),
    ).toEqual(Array(7).fill(false));
  });
});

describe("controlled rule version templates and rollback definitions", () => {
  it.each<RuleTemplate>([
    "special",
    "digits",
    "features",
    "exclude",
    "attributes",
    "m-select-n",
  ])(
    "round-trips the %s template without changing persisted math",
    (template) => {
      const config = defaultRuleVersionForm(template);
      config.unitPoints = "9007199254740993";
      config.odds = "1.000001";
      config.capPoints = "9223372036854775807";
      config.roundingScope = "tier";
      const request = buildRuleSimulationRequest(template, config);
      const restored = restoreRuleVersionTemplate(request.definition);
      expect(restored?.template).toBe(template);
      expect(ruleTemplateModel(template)).toBe(request.definition.model.model);
      const rebuilt = buildRuleSimulationRequest(
        restored!.template,
        restored!.form,
      );
      expect(ruleDefinitionSignature(rebuilt.definition)).toBe(
        ruleDefinitionSignature(request.definition),
      );
      const testCase = buildRuleValidationCase(template, config, {
        name: "完整边界用例",
        bet: "9007199254740993",
        prize: "0",
        won: false,
      });
      expect(testCase.selection).toEqual(request.selection);
      expect(testCase.draw).toEqual(request.draw);
      expect(testCase.multiplier).toBe("1");
      expect(testCase.expected_bet_points).toBe("9007199254740993");
      expect(testCase.expected_won).toBe(false);
    },
  );

  it("handles Go's omitted zero/empty pool fields and null collections losslessly", () => {
    const stored = JSON.parse(
      JSON.stringify(
        buildRuleSimulationRequest("digits", defaultRuleVersionForm("digits"))
          .definition,
      ),
    );
    stored.model.regular_pool = { allow_repeat: false };
    stored.model.special_pool = { allow_repeat: false };
    stored.selection.attribute_groups = null;
    stored.selection.feature_choices = null;
    stored.number_attributes = null;
    expect(restoreRuleVersionTemplate(stored)?.template).toBe("digits");
  });

  it("refuses to guess unsupported definitions or silently discard tiers, changed limits, and nested conditions", () => {
    const nested: RuleDefinition = structuredClone(definition);
    nested.prize_tiers[0].condition = {
      op: "any",
      children: [
        {
          op: "all",
          children: [{ op: "equals", field: "special_match", value: 1 }],
        },
      ],
    };
    expect(restoreRuleVersionTemplate(nested)).toBeNull();
    expect(
      restoreRuleVersionTemplate({
        ...definition,
        limits: { ...definition.limits, max_multiplier: "999" },
      }),
    ).toBeNull();
    expect(
      restoreRuleVersionTemplate({
        ...definition,
        prize_tiers: [...definition.prize_tiers, definition.prize_tiers[0]],
      }),
    ).toBeNull();
  });

  it("accepts a full zero-prize false-won expectation and rejects missing/noncanonical fields and fractional multiplier", () => {
    expect(
      buildRuleValidationCase("special", form, {
        name: "未中奖",
        bet: "4",
        prize: "0",
        won: false,
      }),
    ).toMatchObject({ expected_prize_points: "0", expected_won: false });
    for (const amount of ["", "01", "-1", "+1", "1.5", " 1", "1e2"]) {
      expect(() =>
        buildRuleValidationCase("special", form, {
          name: "用例",
          bet: amount,
          prize: "0",
          won: false,
        }),
      ).toThrow();
    }
    expect(() =>
      buildRuleValidationCase("special", form, {
        name: " ",
        bet: "4",
        prize: "0",
        won: false,
      }),
    ).toThrow();
    expect(() =>
      buildRuleValidationCase(
        "special",
        { ...form, multiplier: "1.5" },
        { name: "用例", bet: "4", prize: "0", won: false },
      ),
    ).toThrow();
  });

  it("locks rollback draft definitions, and allows cloning only the backend's active/expired/rolled_back states", () => {
    expect(ruleVersionDefinitionLocked(version)).toBe(false);
    expect(
      ruleVersionDefinitionLocked({
        ...version,
        source_version_id: "old-active",
      }),
    ).toBe(true);
    for (const status of ["active", "expired", "rolled_back"] as const) {
      const record = { ...version, status };
      expect(ruleVersionDefinitionLocked(record)).toBe(true);
      expect(canCloneRuleVersion(actor, brand, record)).toBe(true);
      expect(
        canCloneRuleVersion({ ...actor, super_admin: true }, brand, record),
      ).toBe(false);
    }
    for (const status of [
      "draft",
      "pending_review",
      "approved",
      "rejected",
    ] as const)
      expect(canCloneRuleVersion(actor, brand, { ...version, status })).toBe(
        false,
      );
  });
});
