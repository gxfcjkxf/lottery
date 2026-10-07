import { readFileSync } from "node:fs";
import { createSSRApp } from "vue";
import * as Vue from "vue";
import { renderToString } from "vue/server-renderer";
import { parse, compileTemplate } from "vue/compiler-sfc";
import { expect, it } from "vitest";
import { createAdminI18n } from "./i18n";

it("preserves a readable separator between exact bet amounts and translated units", async () => {
  // Compile the actual amounts subtree rather than duplicating its markup.
  const source = readFileSync(new URL("./BetOrderManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source).descriptor;
  type Node = { tag?: string; props?: { name?: string; value?: { content: string } }[]; children?: Node[]; loc: { source: string } };
  function find(node: Node): Node | undefined {
    if (node.tag === "div" && node.props?.some(p => p.name === "class" && p.value?.content === "amounts")) return node;
    return node.children?.map(find).find(Boolean);
  }
  const fragment = find(descriptor.template!.ast as unknown as Node);
  expect(fragment).toBeDefined();
  const compiled = compileTemplate({ source: fragment!.loc.source, filename: "BetOrderManagement.vue", id: "amounts-regression" });
  expect(compiled.errors).toEqual([]);
  const code = compiled.code.replace(/^import\s+\{([^}]+)\}\s+from\s+["']vue["'];?\s*$/gm, (_m, bindings: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=Vue;`).replace("export function render", "function render");
  const render = new Function("Vue", `${code}; return render;`)(Vue);
  const i18n = createAdminI18n();
  const amounts = [["倍数", "3"], ["总扣款", "9007199254740993"]];
  const app = () => createSSRApp({ render, setup: () => ({ amountRows: amounts, amountLabel: (label: string) => label, t: i18n.t }) });
  const text = async () => (await renderToString(app())).replace(/<!--[^]*?-->/g, "").replace(/<[^>]+>/g, "");
  expect(await text()).toContain("3 倍"); expect(await text()).toContain("9007199254740993 分");
  i18n.setLocale("en");
  expect(await text()).toContain("3 x"); expect(await text()).toContain("9007199254740993 points");
});
