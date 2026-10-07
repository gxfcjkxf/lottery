import { createSSRApp, h } from "vue";
import { renderToString } from "vue/server-renderer";
import { describe, expect, it } from "vitest";
import RuleCasesEditor from "./RuleCasesEditor.vue";
import RuleConditionEditor from "./RuleConditionEditor.vue";
import RuleDefinitionEditor from "./RuleDefinitionEditor.vue";
import RuleModelEditor from "./RuleModelEditor.vue";
import RuleSimulator from "./RuleSimulator.vue";
import { adminI18nKey, createAdminI18n } from "./i18n";
import { defaultRuleDefinition } from "./rule-editor";
import { defaultRuleCase } from "./rule-cases-editor";
import type { RuleModel } from "./rule-simulation-api";

const model: RuleModel = {
  model: "X_PLUS_Y",
  regular_pool: { min: 1, max: 49, values: [], allow_repeat: false },
  special_pool: { min: 1, max: 49, values: [], allow_repeat: false },
  regular_count: 6,
  special_count: 1,
  pool_size: 0,
  total_count: 0,
  length: 0,
  allow_repeat: false,
  ordered: false,
};
const definition = defaultRuleDefinition(model);
const validationCases = [defaultRuleCase(definition, 1)];

async function render(locale: "en" | "zh-CN", component: Parameters<typeof h>[0], props: Record<string, unknown>) {
  const context = createAdminI18n();
  context.setLocale(locale);
  const app = createSSRApp({ render: () => h(component, props) });
  app.provide(adminI18nKey, context);
  return renderToString(app);
}

describe("rule editor locale rendering", () => {
  it("translates all six actual simulator template labels, keeping their enum values", async () => {
    const brand = "11111111-1111-4111-8111-111111111111";
    const english = await render("en", RuleSimulator, { brandId: brand, account: { id: "22222222-2222-4222-8222-222222222222", super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["rule.simulate.brand"] } } });
    expect(english).toContain("Special-number match"); expect(english).toContain("Straight three-digit play");
    expect(english).not.toMatch(/\p{Script=Han}/u);
    for (const value of ["special", "digits", "features", "exclude", "attributes", "m-select-n"]) expect(english).toContain(`value="${value}"`);
  });
  it("translates the model editor while preserving model enum and selected form values", async () => {
    const before = JSON.stringify(model);
    const english = await render("en", RuleModelEditor, { modelValue: model });
    const chinese = await render("zh-CN", RuleModelEditor, { modelValue: model });

    expect(english).toContain("Custom game number model");
    expect(english).toContain("Number model");
    expect(chinese).toContain("自定义彩种号码模型");
    expect(chinese).toContain("号码模型");
    expect(english).toContain('value="X_PLUS_Y"');
    expect(chinese).toContain('value="X_PLUS_Y"');
    expect(JSON.stringify(model)).toBe(before);
  });

  it("translates nested definition conditions without changing condition AST fields", async () => {
    const before = JSON.stringify(definition);
    const english = await render("en", RuleDefinitionEditor, {
      modelValue: definition,
    });
    const chinese = await render("zh-CN", RuleDefinitionEditor, {
      modelValue: definition,
    });

    expect(english).toContain("Prize tiers and win conditions");
    expect(english).toContain("Condition operator");
    expect(chinese).toContain("奖级与中奖条件");
    expect(chinese).toContain("条件运算");
    expect(english).toContain('value="regular_match"');
    expect(chinese).toContain('value="regular_match"');
    expect(JSON.stringify(definition)).toBe(before);
  });

  it("renders validation-case labels in either locale and preserves user data", async () => {
    const before = JSON.stringify(validationCases);
    const props = { definition, modelValue: validationCases };
    const english = await render("en", RuleCasesEditor, props);
    const chinese = await render("zh-CN", RuleCasesEditor, props);

    expect(english).toContain("Complete validation cases");
    expect(english).toContain("Expected bet points");
    expect(chinese).toContain("完整验证用例");
    expect(chinese).toContain("预期投注积分");
    expect(JSON.stringify(validationCases)).toBe(before);
  });

  it("translates standalone nested conditions but keeps operator and field values stable", async () => {
    const condition = definition.prize_tiers[0]!.condition;
    const before = JSON.stringify(condition);
    const props = { modelValue: condition, definition };
    const english = await render("en", RuleConditionEditor, props);
    const chinese = await render("zh-CN", RuleConditionEditor, props);

    expect(english).toContain("Condition operator");
    expect(english).toContain("Regular number matches");
    expect(chinese).toContain("条件运算");
    expect(chinese).toContain("普通号命中数量");
    expect(JSON.stringify(condition)).toBe(before);
  });
});
