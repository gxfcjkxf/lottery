import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8');
test('user shell has one live betting route and no obsolete demo balances or local orders', () => {
  const source = read('user-web/src/Page.vue');
  assert.match(source, /<BettingPanel\s+v-if="isBettingRoute"/);
  assert.match(source, /<HomeWalletCard/);
  assert.doesNotMatch(source, /placeDemoOrder|readDraft|luma-demo-|12840|Classic 6\/49/);
  assert.match(source, /Platform points · payments not connected/);
});
test('home wallet clears old values and uses the verified wallet reader without fabricated amounts', () => {
  const source = read('user-web/src/HomeWalletCard.vue');
  assert.match(source, /createWalletApi/);
  assert.match(source, /formatIntegerAmount\(wallet.display_points\)/);
  assert.match(source, /wallet.value = null/);
  assert.match(source, /request !== generation/);
  assert.match(source, /onBeforeUnmount/);
  assert.doesNotMatch(source, /Number\(.*points|parseFloat|localStorage|sessionStorage/);
});
