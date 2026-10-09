import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, readFileSync, realpathSync, statSync, writeFileSync, constants, accessSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const tool = process.env.PROMTOOL_BIN;
if (!tool || !isAbsolute(tool) || /[\0\r\n]/.test(tool) || !statSync(realpathSync(tool)).isFile()) throw new Error('PROMTOOL_BIN must name a trusted absolute executable');
accessSync(tool, constants.X_OK);
const generated = mkdtempSync(join(tmpdir(), 'lottery-monitoring-check-'));
const credential = join(generated, 'metrics.token');
writeFileSync(credential, randomBytes(32).toString('base64url'), { flag: 'wx', mode: 0o600 });
const source = readFileSync(join(root, 'monitoring/prometheus.yaml'), 'utf8');
const marker = 'credentials_file: /etc/prometheus/lottery-metrics.token';
if (source.split(marker).length !== 3) throw new Error('Both private jobs must retain their token-file guard');
const prepared = source.replaceAll(marker, `credentials_file: ${JSON.stringify(credential)}`).replace('  - alerts.yaml', `  - ${JSON.stringify(join(root, 'monitoring/alerts.yaml'))}`);
const config = join(generated, 'prometheus.yaml');
writeFileSync(config, prepared, { flag: 'wx', mode: 0o600 });
function verify(args) {
  const result = spawnSync(tool, args, { cwd: resolve(root, 'monitoring'), encoding: 'utf8', timeout: 60_000, maxBuffer: 1_048_576, shell: false });
  if (result.status !== 0 || result.signal || result.error) throw new Error(`Monitoring validation failed at ${args[0]} ${args[1]}`);
}
verify(['check', 'config', config]);
verify(['check', 'rules', 'alerts.yaml']);
verify(['test', 'rules', 'alerts.test.yaml']);
const dashboard = JSON.parse(readFileSync(join(root, 'monitoring/grafana-dashboard.json'), 'utf8'));
if (dashboard.uid !== 'lottery-operations' || dashboard.timezone !== 'utc' || dashboard.panels.length !== 6) throw new Error('Dashboard identity or panel inventory changed');
for (const panel of dashboard.panels) {
  if (!panel.targets?.length || panel.datasource?.type !== 'prometheus') throw new Error('A dashboard panel lacks its typed data source');
  for (const target of panel.targets) verify(['--experimental', 'promql', 'format', target.expr]);
}
// The owned generated fixture remains available for local diagnostics. Never
// overwrite /etc/prometheus or remove production credentials during validation.
console.log('Validated full Prometheus config with owned credentials, 11 alert rules, rule scenarios and six dashboard queries.');
