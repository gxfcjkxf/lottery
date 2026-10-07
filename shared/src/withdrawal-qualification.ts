export interface WithdrawalQualification {
  brand_id: string;
  member_id: string;
  account_id: string;
  base_points: string;
  valid_points: string;
  valid_order_count: string;
  credit_numerator: string;
  credit_denominator: string;
  meets_turnover: boolean;
  cycle_from_at: string | null;
  cycle_from_version: string;
  cutoff_at: string;
  cutoff_version: string;
}

const MAX_INT64 = 9223372036854775807n;
const MAX_DECIMAL_LENGTH = 16384;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const RFC3339 = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|([+-])(\d{2}):(\d{2}))$/;
const KEYS = [
  "brand_id", "member_id", "account_id", "base_points", "valid_points", "valid_order_count",
  "credit_numerator", "credit_denominator", "meets_turnover", "cycle_from_at", "cycle_from_version",
  "cutoff_at", "cutoff_version",
] as const;

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function exactKeys(value: Record<string, unknown>): boolean {
  return Object.keys(value).length === KEYS.length && KEYS.every((key) => Object.hasOwn(value, key));
}

function decimal(value: unknown): value is string {
  return typeof value === "string" && value.length <= MAX_DECIMAL_LENGTH && /^(0|[1-9]\d*)$/.test(value);
}

function int64Decimal(value: unknown): value is string {
  return decimal(value) && value.length <= 19 && BigInt(value) <= MAX_INT64;
}

function dateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = RFC3339.exec(value);
  if (!match) return false;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, , , , offsetHourText, offsetMinuteText] = match;
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  const hour = Number(hourText);
  const minute = Number(minuteText);
  const second = Number(secondText);
  const daysInMonth = month === 2
    ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28)
    : [4, 6, 9, 11].includes(month) ? 30 : 31;
  if (month < 1 || month > 12 || day < 1 || day > daysInMonth || hour > 23 || minute > 59 || second > 59) return false;
  if (offsetHourText !== undefined && (Number(offsetHourText) > 23 || Number(offsetMinuteText) > 59)) return false;
  return Number.isFinite(Date.parse(value));
}

function compareDateTimes(left: string, right: string): number {
  const parts = (value: string) => {
    const match = RFC3339.exec(value)!;
    const [, yearText, monthText, dayText, hourText, minuteText, secondText, fraction = "", zone, sign, offsetHourText = "0", offsetMinuteText = "0"] = match;
    const date = new Date(0);
    date.setUTCFullYear(Number(yearText), Number(monthText) - 1, Number(dayText));
    date.setUTCHours(Number(hourText), Number(minuteText), Number(secondText), 0);
    const offset = zone === "Z" ? 0 : (Number(offsetHourText) * 60 + Number(offsetMinuteText)) * (sign === "+" ? 1 : -1);
    return { seconds: BigInt(Math.trunc(date.getTime() / 1000)) - BigInt(offset * 60), fraction: fraction.replace(/0+$/, "") };
  };
  const a = parts(left);
  const b = parts(right);
  if (a.seconds !== b.seconds) return a.seconds < b.seconds ? -1 : 1;
  const width = Math.max(a.fraction.length, b.fraction.length);
  const af = a.fraction.padEnd(width, "0");
  const bf = b.fraction.padEnd(width, "0");
  return af === bf ? 0 : af < bf ? -1 : 1;
}

export function isWithdrawalQualification(value: unknown): value is WithdrawalQualification {
  if (!record(value) || !exactKeys(value)) return false;
  if (![value.brand_id, value.member_id, value.account_id].every((item) => typeof item === "string" && UUID.test(item))) return false;
  if (!int64Decimal(value.base_points) || !decimal(value.valid_points) || !decimal(value.valid_order_count) ||
      !decimal(value.credit_numerator) || !decimal(value.credit_denominator) || BigInt(value.credit_denominator) === 0n ||
      typeof value.meets_turnover !== "boolean" ||
      !(value.cycle_from_at === null || dateTime(value.cycle_from_at)) ||
      !int64Decimal(value.cycle_from_version) || !dateTime(value.cutoff_at) || !int64Decimal(value.cutoff_version)) return false;
  if ((value.cycle_from_at === null) !== (BigInt(value.cycle_from_version) === 0n)) return false;
  if (BigInt(value.cycle_from_version) > BigInt(value.cutoff_version)) return false;
  if (value.cycle_from_at !== null && compareDateTimes(value.cycle_from_at, value.cutoff_at) > 0) return false;
  return (BigInt(value.credit_numerator) >= BigInt(value.base_points) * BigInt(value.credit_denominator)) === value.meets_turnover;
}
