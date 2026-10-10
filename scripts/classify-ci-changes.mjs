import { execFileSync } from 'node:child_process';
import { appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function documentationOnly(paths) {
  return paths.length > 0 && paths.every(path => path.endsWith('.md') || path === 'tests/static/current-handover.test.mjs');
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { CI_BASE_SHA: base, CI_HEAD_SHA: head, GITHUB_OUTPUT: output } = process.env;
  if (!/^[a-f0-9]{40}$/.test(base ?? '') || !/^[a-f0-9]{40}$/.test(head ?? '') || !output) {
    throw new Error('CI base/head commit SHAs and GITHUB_OUTPUT are required');
  }
  // A first push has no previous commit: always run the complete pipeline.
  let full = true;
  if (base !== '0'.repeat(40)) {
    const paths = execFileSync('git', ['diff', '--no-renames', '--name-only', '-z', base, head], { encoding: 'utf8' }).split('\0').filter(Boolean);
    full = !documentationOnly(paths);
  }
  appendFileSync(output, `business_changed=${full}\n`);
}
