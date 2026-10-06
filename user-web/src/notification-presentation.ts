import type { Language } from "../../shared/src/brand";
import type { NotificationTemplateContent } from "./notification-api";

export type NotificationPresentationItem = {
  event_type: string;
  template_key: string;
  template_version: number;
  content?: NotificationTemplateContent | null;
  payload: { resource_id: string; points: string | null };
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

const copy = {
  en: {
    "member.joined": {
      title: "Welcome",
      body: "Your membership is ready. Welcome aboard.",
    },
    "recharge.confirmed": {
      title: "Recharge confirmed",
      body: "Recharge confirmed: {points} points.",
    },
    "bet.order.placed": {
      title: "Order submitted",
      body: "Order submitted: {points} points.",
    },
    "bet.order.cancelled": {
      title: "Order cancelled",
      body: "Cancelled. {points} points were returned to your original balance.",
    },
    "bet.order.judged_cancelled": {
      title: "Order cancelled after review",
      body: "Cancelled after review. {points} points were returned to your original balance.",
    },
    "bet.order.abnormal": {
      title: "Order needs review",
      body: "Your order needs manual review. Points involved: {points}.",
    },
    "bet.order.won": {
      title: "Prize credit recorded",
      body: "Historical record: {points} points were credited as this order's prize. This records the credit, not your current wallet balance or a guaranteed final outcome. Any correction will appear as a separate prize event; this record is retained.",
    },
    "bet.order.prize_reversed": {
      title: "Prize reversal recorded",
      body: "Historical record: the full original prize amount of {points} points for this order was reversed. This records the reversal, not your current wallet balance. Any later prize correction will appear as a separate event; this record is retained.",
    },
  },
  zh: {
    "member.joined": { title: "欢迎", body: "您的会员账户已准备就绪，欢迎加入。" },
    "recharge.confirmed": { title: "充值已确认", body: "充值已确认：{points} 积分。" },
    "bet.order.placed": { title: "注单已提交", body: "注单已提交，涉及 {points} 积分。" },
    "bet.order.cancelled": {
      title: "注单已取消",
      body: "注单已取消，{points} 积分已原路退回。",
    },
    "bet.order.judged_cancelled": {
      title: "注单已判定取消",
      body: "注单经判定已取消，{points} 积分已原路退回。",
    },
    "bet.order.abnormal": {
      title: "注单待人工处理",
      body: "您的注单需要人工处理，涉及积分：{points}。",
    },
    "bet.order.won": {
      title: "派奖入账记录",
      body: "历史记录：此注单的 {points} 积分奖金已记入账本。此记录仅表示该笔入账，不代表当前钱包余额，也不保证最终结果。任何更正都会作为单独的奖金事件记录；此记录会保留。",
    },
    "bet.order.prize_reversed": {
      title: "奖金冲正记录",
      body: "历史记录：此注单原奖金全额 {points} 积分已冲回。此记录仅表示该笔冲正，不代表当前钱包余额。之后如有奖金更正，会作为单独事件记录；此记录会保留。",
    },
  },
} as const;

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
  const known = copy[locale][item.event_type as keyof (typeof copy)[typeof locale]];
  if (!known) throw new RangeError(`Unsupported notification event: ${item.event_type}`);
  if ((item.event_type === "member.joined") !== (item.payload.points === null)) {
    throw new RangeError(`Invalid points value for notification event: ${item.event_type}`);
  }
  const points = item.payload.points === null
    ? null
    : formatIntegerString(item.payload.points, locale);
  const snapshot = item.content ?? null;
  if (snapshot === null && item.template_version !== 1) {
    throw new RangeError(`Missing notification template snapshot for version: ${item.template_version}`);
  }
  const source = snapshot
    ? snapshot[locale === "zh" ? "zh-CN" : "en"]
    : known;
  const replacePlaceholders = (value: string) => value
    .replaceAll("{points}", points ?? "")
    .replaceAll("{resource_id}", item.payload.resource_id);
  const title = snapshot ? replacePlaceholders(source.title) : source.title;
  const body = snapshot
    ? replacePlaceholders(source.body)
    : item.payload.points !== null
      ? known.body.replaceAll("{points}", points ?? item.payload.points)
      : known.body;
  const protectedNote = snapshot && (item.event_type === "bet.order.won" || item.event_type === "bet.order.prize_reversed")
    ? known.body.replaceAll("{points}", points ?? "")
    : null;
  const date = new Date(item.created_at);
  const createdAt = Number.isNaN(date.getTime())
    ? item.created_at
    : new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
  return {
    ...known,
    title,
    body,
    createdAt,
    points,
    reference: item.event_type === "member.joined" ? null : item.payload.resource_id,
    protectedNote,
  };
}
