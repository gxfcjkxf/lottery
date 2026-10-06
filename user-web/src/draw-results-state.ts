import type { PublicDrawResult } from "./draws-api";

export interface ResultNumberGroup {
  key: "regular" | "special" | "digits";
  label: string;
  positions: number[];
}

/** Route/game identifiers are filters only after they pass canonical UUID syntax. */
export function isUuid(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
      value,
    )
  );
}

/** Period numbers are exact UTF-8 strings; do not trim or otherwise normalize them. */
export function isValidPeriodFilter(value: string): boolean {
  return (
    value.length === 0 ||
    (value.trim().length > 0 && new TextEncoder().encode(value).length <= 80)
  );
}

export function shiftedPageOffset(
  current: number,
  direction: -1 | 1,
  pageSize: number,
): number {
  return Math.max(0, current + direction * pageSize);
}

export function resultNumberGroups(
  result: PublicDrawResult["result"],
  locale: "zh" | "en",
): ResultNumberGroup[] {
  const groups: ResultNumberGroup[] = [];
  if (result.regular.length) {
    groups.push({
      key: "regular",
      label: locale === "zh" ? "普通号码" : "Regular",
      positions: [...result.regular],
    });
  }
  if (result.special.length) {
    groups.push({
      key: "special",
      label: locale === "zh" ? "特别号码" : "Special",
      positions: [...result.special],
    });
  }
  if (result.digits.length) {
    groups.push({
      key: "digits",
      label: locale === "zh" ? "位置数字" : "Position digits",
      // Digit order and repetition are meaningful, and zero is a real digit.
      positions: [...result.digits],
    });
  }
  return groups;
}

export function formatDrawDateTime(
  value: string,
  timezone: string,
  locale: "zh" | "en",
): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return value;
  try {
    return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
      dateStyle: "medium",
      timeStyle: "medium",
      timeZone: timezone,
    }).format(date);
  } catch {
    return date.toISOString();
  }
}

export function isCancelledPeriodStatus(status: string): boolean {
  return [
    "bet_cancelled",
    "judged_cancelled",
    "cancelled",
    "canceled",
  ].includes(status);
}

export function periodStatusLabel(status: string, locale: "zh" | "en"): string {
  const labels: Record<string, { zh: string; en: string }> = {
    pending: { zh: "待开始", en: "Pending" },
    betting: { zh: "销售中", en: "Betting open" },
    closed: { zh: "销售已截止", en: "Betting closed" },
    waiting_draw: { zh: "等待开奖", en: "Waiting for draw" },
    drawn: { zh: "已开奖", en: "Drawn" },
    settling: { zh: "结算处理中", en: "Settlement in progress" },
    settled: { zh: "已结算", en: "Settled" },
    bet_cancelled: {
      zh: "投注已取消：结果仅作记录，不用于有效判定或派奖",
      en: "Bet cancelled: result is for record only, not a valid outcome or payout",
    },
    judged_cancelled: {
      zh: "期次已取消：结果仅作记录，不用于有效判定或派奖",
      en: "Period cancelled: result is for record only, not a valid outcome or payout",
    },
    cancelled: {
      zh: "已取消：结果仅作记录，不用于有效判定或派奖",
      en: "Cancelled: result is for record only, not a valid outcome or payout",
    },
    canceled: {
      zh: "已取消：结果仅作记录，不用于有效判定或派奖",
      en: "Cancelled: result is for record only, not a valid outcome or payout",
    },
  };
  return labels[status]?.[locale] ?? status;
}
