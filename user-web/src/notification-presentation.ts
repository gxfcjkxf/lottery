import type { Language } from "../../shared/src/brand";

export type NotificationPresentationItem = {
  event_type: string;
  template_key: string;
  template_version: number;
  payload: { resource_id: string; points: string | null };
  created_at: string;
};

export type NotificationPresentation = {
  title: string;
  body: string;
  createdAt: string;
  points: string | null;
  reference: string | null;
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
  if (item.template_version !== 1) {
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
  const body = item.payload.points !== null
    ? known.body.replace("{points}", points ?? item.payload.points)
    : known.body;
  const date = new Date(item.created_at);
  const createdAt = Number.isNaN(date.getTime())
    ? item.created_at
    : new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(date);
  return {
    ...known,
    body,
    createdAt,
    points,
    reference: item.event_type === "member.joined" ? null : item.payload.resource_id,
  };
}
