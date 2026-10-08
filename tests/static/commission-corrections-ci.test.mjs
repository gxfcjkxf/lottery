import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

test('commission correction CI uses isolated viewport databases and real no-retry financial UI work', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/ci.yaml', import.meta.url), 'utf8');
  const job = /^  commission-corrections-browser:\n([\s\S]*?)(?=^  withdrawal-browser:)/m.exec(workflow)?.[1];
  assert.ok(job);
  assert.match(job, /viewport: \[desktop, mobile\]/);
  assert.match(job, /POSTGRES_DB: lottery_commission_correction_\$\{\{ matrix\.viewport \}\}_s27/);
  assert.match(job, /COMMISSION_FIXTURE_CONFIRM: owned_synthetic_database/);
  assert.match(job, /\.\/bin\/commission-fixture init/);
  assert.match(job, /commission-corrections\.spec\.ts --project=\$\{\{ matrix\.viewport \}\} --workers=1 --retries=0/);
  assert.match(job, /stats\.skipped !== 0/);
  assert.match(job, /stats\.expected > 0/);
  assert.doesNotMatch(job, /\.\/bin\/platform worker/);
  const spec = readFileSync(new URL('../browser/commission-corrections.spec.ts', import.meta.url), 'utf8');
  for (const operation of ['prepare-corrections', 'execute-corrections', 'freeze-corrections', 'unfreeze-corrections', 'verify-corrections', 'notify']) assert.ok(spec.includes(operation));
  assert.match(spec, /x-commission-correction-actor-id/);
  assert.match(spec, /expect\(writes\[1\]\)\.toEqual\(writes\[0\]\)/);
  assert.match(spec, /economic_fingerprint/);
  assert.match(spec, /commission-report-export/);
  assert.match(spec, /x-report-format-version/);
  assert.match(spec, /commission\.corrected/);
  assert.match(spec, /negative points record a past recovery/);
  assert.match(spec, /messages\)\.toHaveLength\(1\)/);
  assert.match(spec, /items\.find\(item => item\.id === message\.id\)\)\.toEqual\(message\)/);
});
