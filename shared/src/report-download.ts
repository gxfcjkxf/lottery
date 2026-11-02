/** Read a completed CSV export. Business totals and CSV construction belong to the server. */
export async function readReportDownload(response: Response, brandId: string, kind: string, invalid: () => never) {
  const headers = response.headers;
  const snapshotAt = headers.get('X-Report-Snapshot-At');
  const groupCount = headers.get('X-Report-Group-Count');
  const auditLogId = headers.get('X-Report-Audit-ID');
  if (response.status !== 200 || headers.get('Content-Type')?.split(';')[0].trim().toLowerCase() !== 'text/csv' ||
      headers.get('X-Report-Brand-ID')?.toLowerCase() !== brandId.toLowerCase() || headers.get('X-Report-Kind') !== kind ||
      !snapshotAt || !Number.isFinite(Date.parse(snapshotAt)) || !groupCount || !/^(0|[1-9]\d*)$/.test(groupCount) ||
      !auditLogId || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(auditLogId)) return invalid();
  const bytes = new Uint8Array(await response.arrayBuffer());
  if (!bytes.length || bytes.length > 4 * 1024 * 1024) return invalid();
  const timestamp = new Date(snapshotAt).toISOString().slice(0, 19).replace(/[-:]/g, '') + 'Z';
  return { filename: `lottery-${kind}-${brandId.toLowerCase()}-${timestamp}.csv`, bytes, groupCount, snapshotAt, auditLogId };
}
