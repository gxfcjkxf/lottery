import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const source = readFileSync(new URL("./App.vue", import.meta.url), "utf8");

describe("admin login entry", () => {
  it("keeps the complete app shell behind a resolved brand-admin session", () => {
    expect(source).toMatch(
      /<div v-if="authLoading \|\| !account" class="login-entry">[\s\S]*?<div v-else class="app-shell"/,
    );
    expect(source).toContain("class=\"login-entry__loading\" role=\"status\"");
  });

  it("offers the real account and password login form in the entry view", () => {
    expect(source).toContain("<form class=\"login-entry__form\" @submit.prevent=\"login\">");
    expect(source).toContain("autocomplete=\"username\"");
    expect(source).toContain("autocomplete=\"current-password\"");
    expect(source).toContain("登录并加载真实成员");
    expect(source).toContain("data-testid=\"admin-language\"");
  });

  it("keeps platform super-admin identities out of the brand console", () => {
    expect(source.match(/if \(result\.account\.super_admin\)/g)).toHaveLength(2);
    expect(source).toContain("error.code === \"ADMIN_ENTRY_MISMATCH\"");
    expect(source).toContain("平台超级管理员请使用平台管理入口。");
    expect(source).not.toContain("platformAdminUrl");
    expect(source).not.toContain("localhost:5175");
    expect(source).not.toContain("login-entry__platform-link");
  });
});
