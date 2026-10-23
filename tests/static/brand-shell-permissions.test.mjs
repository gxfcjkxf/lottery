import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
test('brand member controls use mapped brand grants without legacy flat permissions', () => {
  const source = readFileSync(new URL('../../admin-web/src/App.vue', import.meta.url), 'utf8');
  const helper = source.split('const hasPermission =')[1].split('const canEditUsers')[0];
  assert.match(helper, /brandPermissionSet\(account.value, selectedBrandId.value\).has\(permission\)/);
  assert.doesNotMatch(helper, /account.value.permissions/);
  assert.doesNotMatch(source, /permissions_by_brand\s*\?\?\s*account.value\?\.permissions/);
  assert.match(source, /:disabled="!canEditUsers && !canResetPasswords"/);
  assert.match(source, /v-if="m.online && canKickUsers"[^>]*@click="openKick\(m\)"/);
  assert.match(source, /v-if="canResetPasswords"[^>]*@click="openReset\(editTarget\)"/);
  assert.match(source, /:disabled="!canEditUsers"/);
});
