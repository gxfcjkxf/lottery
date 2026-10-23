import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const credentials = readFileSync(new URL("../../shared/src/LoginCredentials.vue", import.meta.url), "utf8");
const source = readFileSync(new URL("./App.vue", import.meta.url), "utf8");

describe("admin login entry", () => {
  it("keeps the complete app shell behind a resolved brand-admin session", () => {
    expect(source).toMatch(
      /<LoginEntry v-if="authLoading \|\| !account"[\s\S]*?<ManagementFrame v-else/,
    );
    expect(source).toContain(':ready="!authLoading"');
    const frame = readFileSync(new URL('../../shared/src/ManagementFrame.vue',import.meta.url),'utf8');
    expect(frame).toContain('class="app-shell management-console"');
    expect(frame).toContain('<aside class="sidebar">');
    expect(frame).toContain('<main class="main-shell">');
  });

  it("offers the real account and password login form in the entry view", () => {
    expect(source).toMatch(/<form[^>]*class="login-entry__form"[^>]*@submit\.prevent="withGlobalLoading\(login\)"/);
    expect(credentials).toContain("autocomplete=\"username\"");
    expect(credentials).toContain("autocomplete=\"current-password\"");
    expect(source).toContain("admin.copy.a182c1e1");
    expect(source).toContain("language-test-id=\"admin-language\"");
  });

  it("keeps platform super-admin identities out of the brand console", () => {
    expect(source.match(/if \(result\.account\.super_admin\)/g)).toHaveLength(2);
    expect(source).toContain("error.code === \"ADMIN_ENTRY_MISMATCH\"");
    expect(source).toContain("admin.copy.51a5468e");
    expect(source).not.toContain("platformAdminUrl");
    expect(source).not.toContain("localhost:5175");
    expect(source).not.toContain("login-entry__platform-link");
  });
});
