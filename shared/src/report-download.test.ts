import { describe, expect, it } from 'vitest';
import { readReportDownload } from './report-download';
const brand = '11111111-1111-4111-8111-111111111111';
const audit = '22222222-2222-4222-8222-222222222222';
const invalid = (): never => { throw new Error('Invalid export'); };
function response(patch: Record<string, string> = {}, status = 200) {
  return new Response('server-generated,csv\r\nvalue,9223372036854775808\r\n', { status, headers: {
    'Content-Type': 'text/csv; charset=utf-8', 'X-Report-Brand-ID': brand, 'X-Report-Kind': 'betting',
    'X-Report-Snapshot-At': '2026-10-27T01:02:03Z', 'X-Report-Group-Count': '1', 'X-Report-Audit-ID': audit, ...patch,
  } });
}
describe('CSV downloads', () => {
  it('preserves server bytes and does not reparse business totals or require a digest', async () => {
    const reply = response(), expected = new Uint8Array(await reply.clone().arrayBuffer());
    const file = await readReportDownload(reply, brand, 'betting', invalid);
    expect(file.bytes).toEqual(expected);
    expect(file.filename).toBe(`lottery-betting-${brand}-20261027T010203Z.csv`);
    expect(file.groupCount).toBe('1');
  });
  it('rejects cross-brand files, wrong report kinds, non-CSV and partial responses', async () => {
    const cases: Record<string, string>[] = [{ 'X-Report-Brand-ID': audit }, { 'X-Report-Kind': 'ledger' }, { 'Content-Type': 'text/html' }];
    for (const patch of cases) {
      await expect(readReportDownload(response(patch), brand, 'betting', invalid)).rejects.toThrow('Invalid export');
    }
    await expect(readReportDownload(response({}, 206), brand, 'betting', invalid)).rejects.toThrow('Invalid export');
  });
});
