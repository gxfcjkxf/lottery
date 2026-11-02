import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');
test('user shell has one live betting route and no obsolete demo balances or local orders', () => {
  const source = read('user-web/src/Page.vue');
  const catalog = read('user-web/src/i18n-catalog-ui.ts');
  assert.match(source, /<ConfiguredBetting\s+v-if="currentGame"/);
  assert.match(source, /class="workspace-game-trigger"/);
  assert.match(source, /loadWorkspaceGames/);
  assert.match(source, /:game-id="currentGame.id"/);
  assert.doesNotMatch(source, /class="home-lobby"/);
  assert.doesNotMatch(source, /<HomeWalletCard|class="hero"|class="lower-grid"/);
  assert.doesNotMatch(source, /placeDemoOrder|readDraft|luma-demo-|12840|Classic 6\/49/);
  assert.match(catalog, /Platform points · payments not connected/);
});
