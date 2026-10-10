import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';

const project = process.argv[2];
if (process.argv.length !== 3 || !['tablet768', 'laptop1024'].includes(project)) {
  throw new Error('Usage: pnpm test:commission-responsive tablet768|laptop1024; initialize a separate owned fixture database for each project first');
}
const cli = createRequire(import.meta.url).resolve('@playwright/test/cli');
// These scenarios deliberately advance the same genuine commission workflow.
// Run files in business order, not Playwright's alphabetical discovery order.
for (const file of [
  'commission-cycles.spec.ts',
  'commission-payments.spec.ts',
  'commission-adjustments.spec.ts',
  'commission-reports.spec.ts',
  'commission-analysis.spec.ts',
]) {
  const result = spawnSync(process.execPath, [cli, 'test', `tests/browser/${file}`, '--config', 'playwright.commission-responsive.config.ts', `--project=${project}`], {
    env: process.env, stdio: 'inherit', timeout: 180_000,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${file} failed: status=${result.status}, signal=${result.signal}`);
}
