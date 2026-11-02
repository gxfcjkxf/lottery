export interface ArchiveSchedule {
  daily_execution_time: string;
  daily_period_offset: number;
  monthly_execution_day: number;
  monthly_execution_time: string;
  monthly_period_offset: number;
}
export interface ArchiveScheduledRun { execute_at: string; period_key: string }
export interface ArchiveNextRuns { daily: ArchiveScheduledRun | null; monthly: ArchiveScheduledRun | null }
export const archiveScheduleFields = ['daily_execution_time', 'daily_period_offset', 'monthly_execution_day', 'monthly_execution_time', 'monthly_period_offset'] as const;
export function archiveScheduleOf(value: ArchiveSchedule): ArchiveSchedule {
  return Object.fromEntries(archiveScheduleFields.map(key => [key, value[key]])) as unknown as ArchiveSchedule;
}
export function sameArchiveSchedule(a: ArchiveSchedule, b: ArchiveSchedule): boolean {
  return archiveScheduleFields.every(key => a[key] === b[key]);
}
export function validArchiveSchedule(value: unknown): value is ArchiveSchedule {
  if (!value || typeof value !== 'object') return false;
  const v = value as ArchiveSchedule;
  const time = (x: unknown) => typeof x === 'string' && /^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(x);
  const integer = (x: unknown, max: number) => typeof x === 'number' && Number.isInteger(x) && x >= 1 && x <= max;
  return time(v.daily_execution_time) && time(v.monthly_execution_time) && integer(v.daily_period_offset, 365) && integer(v.monthly_period_offset, 120) && integer(v.monthly_execution_day, 31);
}
export function validArchiveNextRuns(value: unknown): value is ArchiveNextRuns {
  if (!value || typeof value !== 'object' || Object.keys(value).sort().join(',') !== 'daily,monthly') return false;
  return (['daily','monthly'] as const).every(kind => {
    const run = (value as ArchiveNextRuns)[kind];
    return run === null || Boolean(run && typeof run === 'object' && Object.keys(run).sort().join(',') === 'execute_at,period_key' &&
      typeof run.execute_at === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(run.execute_at) && Number.isFinite(Date.parse(run.execute_at)) &&
      (kind === 'daily' ? /^\d{4}-(0[1-9]|1[0-2])-([0-2][0-9]|3[01])$/ : /^\d{4}-(0[1-9]|1[0-2])$/).test(run.period_key));
  });
}
