import type { Language } from "../../shared/src/brand";
import { formatDisplayDateTime } from '@lottery/shared';
import type { DrawNotificationPayload, NotificationEventType, NotificationTemplateContent } from "./notification-api";
import { userUiCatalog } from './i18n-catalog-ui';

export type NotificationPresentationItem = {
  event_type: NotificationEventType;
  template_key: NotificationEventType;
  template_version: number;
  content: NotificationTemplateContent;
  payload: { resource_id: string; points: string | null } | DrawNotificationPayload;
  created_at: string;
};

export type NotificationPresentation = {
  title: string;
  body: string;
  createdAt: string;
  points: string | null;
  reference: string | null;
  protectedNote: string | null;
};

const supportedEvents: readonly NotificationEventType[] = [
  'draw.result.published', 'draw.result.corrected', 'member.joined', 'recharge.confirmed',
  'bet.order.placed', 'bet.order.cancelled', 'bet.order.judged_cancelled', 'bet.order.abnormal', 'bet.order.won', 'bet.order.prize_reversed',
  'reward.order.granted', 'reward.order.revocation_pending', 'reward.order.revoked',
  'commission.paid', 'commission.adjusted', 'commission.corrected',
  'withdrawal.order.reviewing', 'withdrawal.order.processing', 'withdrawal.order.paid', 'withdrawal.order.rejected', 'withdrawal.order.failed', 'withdrawal.order.cancelled',
];

/** Group an integer string without converting it to a lossy JS number. */
export function formatIntegerString(value: string | null, _locale: Language): string {
  if (value === null) return "";
  const match = /^([+-]?)(\d+)$/.exec(value);
  if (!match) return value;
  const [, sign, digits] = match;
  const grouped = digits.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return `${sign}${grouped}`;
}

/** Render only supported, versioned notification facts; unknown variants fail closed. */
export function renderNotification(
  item: NotificationPresentationItem,
  locale: Language,
): NotificationPresentation {
  if (!Number.isSafeInteger(item.template_version) || item.template_version < 1) {
    throw new RangeError(`Unsupported notification template version: ${item.template_version}`);
  }
  if (item.template_key !== item.event_type) {
    throw new RangeError(`Unsupported notification template: ${item.template_key}`);
  }
  if (!supportedEvents.includes(item.event_type)) throw new RangeError(`Unsupported notification event: ${item.event_type}`);
  if (!item.content) throw new RangeError("Notification requires an immutable content snapshot");
  const messages: Record<string, string> = userUiCatalog[locale === 'zh' ? 'zh-CN' : 'en'];
  const source = item.content[locale === "zh" ? "zh-CN" : "en"];
  const translate = (key: string, parameters: Record<string, string>) => messages[key].replace(/\{([A-Za-z][A-Za-z0-9_]*)\}/g, (placeholder, name: string) => parameters[name] ?? placeholder);
  const isDraw = item.event_type === "draw.result.published" || item.event_type === "draw.result.corrected";
  if (isDraw) {
    if (item.payload.points !== null || !("draw" in item.payload)) {
      throw new RangeError("Draw notifications require null points, draw facts, and an immutable content snapshot");
    }
    const payload = item.payload as DrawNotificationPayload;
    const draw = payload.draw;
    const empty = messages['user.notification.none'];
    const regular = draw.result.regular.length ? draw.result.regular.join(", ") : empty;
    const special = draw.result.special.length ? draw.result.special.join(", ") : empty;
    const digits = draw.result.digits.length ? draw.result.digits.join("") : empty;
    const protectedNote = translate('user.notification.drawNote', { period: draw.period_no, regular, special, digits, drawnAt: formatDisplayDateTime(draw.drawn_at, locale) });
    const replace = (text: string) => text.replaceAll("{resource_id}", payload.resource_id);
    return {
      title: replace(source.title),
      body: replace(source.body),
      createdAt: formatDisplayDateTime(item.created_at, locale),
      points: null,
      reference: payload.resource_id,
      protectedNote,
    };
  }
  if ((item.event_type === "member.joined") !== (item.payload.points === null)) {
    throw new RangeError(`Invalid points value for notification event: ${item.event_type}`);
  }
  const points = item.payload.points === null
    ? null
    : formatIntegerString(item.payload.points, locale);
  const withdrawalEvent = item.event_type.startsWith("withdrawal.order.");
  const commissionEvent = item.event_type === "commission.paid" || item.event_type === "commission.adjusted" || item.event_type === "commission.corrected";
  const rewardEvent = item.event_type.startsWith("reward.order.");
  const replacePlaceholders = (value: string) => value
    .replaceAll("{points}", points ?? "")
    .replaceAll("{resource_id}", item.payload.resource_id);
  const title = replacePlaceholders(source.title);
  const body = replacePlaceholders(source.body);
  const protectedNote = withdrawalEvent || commissionEvent || rewardEvent || item.event_type === "bet.order.won" || item.event_type === "bet.order.prize_reversed"
    ? (() => {
        const params = { points: points ?? "" };
        if (withdrawalEvent) {
          const status = item.event_type.slice("withdrawal.order.".length);
          const statusKey = `user.notification.withdrawal.${status}`;
          return translate('user.notification.withdrawalNote', { ...params, status: messages[statusKey] ?? status });
        }
        const key = item.event_type === "bet.order.won" ? "user.notification.betWonNote"
          : item.event_type === "bet.order.prize_reversed" ? "user.notification.betReversedNote"
          : item.event_type === "reward.order.granted" ? "user.notification.rewardGrantedNote"
          : item.event_type === "reward.order.revocation_pending" ? "user.notification.rewardPendingNote"
          : item.event_type === "reward.order.revoked" ? "user.notification.rewardRevokedNote"
          : item.event_type === "commission.paid" ? "user.notification.commissionPaidNote"
          : item.event_type === "commission.adjusted" ? "user.notification.commissionAdjustedNote"
          : "user.notification.commissionCorrectedNote";
        return translate(key, params);
      })()
    : null;
  const createdAt = formatDisplayDateTime(item.created_at, locale);
  return {
    ...source,
    title,
    body,
    createdAt,
    points,
    reference: item.event_type === "member.joined" ? null : item.payload.resource_id,
    protectedNote,
  };
}
