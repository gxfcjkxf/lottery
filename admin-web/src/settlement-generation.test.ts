import { readFileSync } from "node:fs";
import { createSSRApp } from "vue";
import * as Vue from "vue";
import { renderToString } from "vue/server-renderer";
import { compileTemplate, parse } from "vue/compiler-sfc";
import { expect, it } from "vitest";
import { createAdminI18n } from "./i18n";

it("renders the exact generation and current or historical label in each locale", async () => {
  const source = readFileSync(new URL("./SettlementManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source).descriptor;
  type Node = { tag?: string; props?: { name?: string; value?: { content: string } }[]; children?: Node[]; loc: { source: string } };
  function find(node: Node): Node | undefined {
    if (node.tag === "div" && node.loc.source.startsWith("<div><dt>{{ t(\"结算代次 / 是否当前\"")) return node;
    return node.children?.map(find).find(Boolean);
  }
  const fragment = find(descriptor.template!.ast as unknown as Node);
  expect(fragment).toBeDefined();
  const compiled = compileTemplate({ source: fragment!.loc.source, filename: "SettlementManagement.vue", id: "settlement-generation-regression" });
  expect(compiled.errors).toEqual([]);
  const code = compiled.code.replace(/^import\s+\{([^}]+)\}\s+from\s+["']vue["'];?\s*$/gm, (_m, bindings: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=Vue;`).replace("export function render", "function render");
  const render = new Function("Vue", `${code}; return render;`)(Vue);
  const i18n = createAdminI18n();
  const selectedJob = { generation: 2, current: true };
  const app = () => createSSRApp({ render, setup: () => ({ selectedJob, t: i18n.t }) });
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
