import type { PublicDrawResult } from "./draws-api";
import { formatDisplayDateTime } from '@lottery/shared';
import { userUiCatalog } from './i18n-catalog-ui';

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
  const messages: Record<string, string> = userUiCatalog[locale === 'zh' ? 'zh-CN' : 'en'];
  const groups: ResultNumberGroup[] = [];
  if (result.regular.length) {
    groups.push({
      key: "regular",
      label: messages['user.draw.group.regular'],
      positions: [...result.regular],
    });
  }
  if (result.special.length) {
    groups.push({
      key: "special",
      label: messages['user.draw.group.special'],
      positions: [...result.special],
    });
  }
  if (result.digits.length) {
    groups.push({
      key: "digits",
      label: messages['user.draw.group.digits'],
      // Digit order and repetition are meaningful, and zero is a real digit.
      positions: [...result.digits],
    });
  }
  return groups;
}

export function formatDrawDateTime(
  value: string,
  locale: "zh" | "en",
): string {
  return formatDisplayDateTime(value, locale);
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
  const messages: Record<string, string> = userUiCatalog[locale === 'zh' ? 'zh-CN' : 'en'];
  const keys: Record<string, string> = {
    pending: 'user.draw.state.pending', betting: 'user.draw.state.betting', closed: 'user.draw.state.closed', waiting_draw: 'user.draw.state.waitingDraw', drawn: 'user.draw.state.drawn', settling: 'user.draw.state.settling', settled: 'user.draw.state.settled', bet_cancelled: 'user.draw.state.betCancelled', judged_cancelled: 'user.draw.state.judgedCancelled', cancelled: 'user.draw.state.cancelled', canceled: 'user.draw.state.cancelled',
  };
  const key = keys[status];
  return key ? messages[key] : status;
}
