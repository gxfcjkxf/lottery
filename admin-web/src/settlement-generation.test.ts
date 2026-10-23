import { readFileSync } from "node:fs";
import { createSSRApp, h } from "vue";
import { renderToString } from "vue/server-renderer";
import { expect, it } from "vitest";
import { createAdminI18n } from "./i18n";

it("renders the exact generation and current or historical label in each locale", async () => {
  const source = readFileSync(new URL("./SettlementManagement.vue", import.meta.url), "utf8");
  expect(source).toContain("admin.settlement.generation");
  const i18n = createAdminI18n();
  const selectedJob = { generation: 2, current: true };
  const app = () => createSSRApp({ render: () => h("div", [i18n.t("admin.settlement.generation", { generation: selectedJob.generation }), " · ", selectedJob.current ? i18n.t("admin.copy.a83d9ef8") : i18n.t("admin.copy.5af7459d")]) });
  const text = async () => (await renderToString(app())).replace(/<!--[^]*?-->/g, "").replace(/<[^>]+>/g, "");

  expect(typeof selectedJob.generation).toBe("number");
  expect(selectedJob.generation).toBe(2);
  expect(await text()).toContain("第 2 代 · 当前");
  i18n.setLocale("en");
  const englishCurrent = await text();
  expect(englishCurrent).toContain("Generation 2 · Current");
  expect(englishCurrent).not.toMatch(/[\u3400-\u9fff]/);

  selectedJob.current = false;
  i18n.setLocale("zh-CN");
  expect(await text()).toContain("第 2 代 · 历史 / 更正冻结");
  i18n.setLocale("en");
  expect(await text()).toContain("Generation 2 · Historical / frozen by correction");
});
