import type { Language } from "../../shared/src/brand";
import type { NotificationEventType, NotificationTemplateContent } from "./notification-api";

export type NotificationPresentationItem = {
  event_type: NotificationEventType;
  template_key: NotificationEventType;
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
    "reward.order.granted": {
      title: "Gift points credited",
      body: "Historical record: {points} points were credited to your gift available balance. This records that past credit, not your current balance or an external payment.",
    },
    "reward.order.revocation_pending": {
      title: "Reward revocation pending recorded",
      body: "Historical record: a full reversal of {points} gift points was requested and recorded as awaiting operator handling. No points moved in this attempt. There is no automatic retry, debit, or unfreeze. This is not your current state, balance or an external payment.",
    },
    "reward.order.revoked": {
      title: "Gift points reversal recorded",
      body: "Historical record: the full original gift of {points} available points was reversed. This records the reversal, not your current balance or any external payment.",
    },
    "commission.paid": { title: "Commission credit recorded", body: "Historical record: {points} points were credited to your commission wallet. This records a past credit, not new income or an external payment forecast. Check your current wallet balance; this record is retained." },
    "commission.adjusted": { title: "Commission adjustment recorded", body: "Historical record: a commission adjustment of {points} points was recorded. This records a past adjustment, not new income or an external payment forecast. Check your current wallet balance; this record is retained." },
    "commission.corrected": { title: "Commission correction recorded", body: "Historical record: a commission correction of {points} points was posted to your commission available balance. Positive points record a past additional credit; negative points record a past recovery. This is not your current balance, new income or an external payment. Check your current wallet; this record is retained." },
    "withdrawal.order.reviewing": { title: "Withdrawal status recorded", body: "Historical withdrawal status: reviewing. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state." },
    "withdrawal.order.processing": { title: "Withdrawal status recorded", body: "Historical withdrawal status: processing. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state." },
    "withdrawal.order.paid": { title: "Withdrawal status recorded", body: "Historical withdrawal status: paid. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state." },
    "withdrawal.order.rejected": { title: "Withdrawal status recorded", body: "Historical withdrawal status: rejected. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state." },
    "withdrawal.order.failed": { title: "Withdrawal status recorded", body: "Historical withdrawal status: failed. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state." },
    "withdrawal.order.cancelled": { title: "Withdrawal status recorded", body: "Historical withdrawal status: cancelled. Points involved: {points}. This is an internal points record, not proof of an external transfer. Check the withdrawal order for its current state." },
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
    "reward.order.granted": {
      title: "赠送积分已入账",
      body: "历史记录：{points} 积分曾记入赠送可用积分。此记录仅表示过去的入账，不代表当前余额或外部付款。",
    },
    "reward.order.revocation_pending": {
      title: "奖励撤销待处理记录",
      body: "历史记录：曾申请全额撤销 {points} 赠送积分，并记录为待运营处理。本次没有积分变动，不会自动重试、扣款或解冻。此记录不代表当前状态、当前余额或外部付款。",
    },
    "reward.order.revoked": {
      title: "赠送积分冲回记录",
      body: "历史记录：原始可用赠送积分 {points} 已全额冲回。此记录仅表示该笔冲回，不代表当前余额或任何外部付款。",
    },
    "commission.paid": { title: "佣金入账记录", body: "历史记录：{points} 积分曾记入佣金钱包。此记录表示过去的入账，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。" },
    "commission.adjusted": { title: "佣金调整记录", body: "历史记录：曾调整 {points} 积分。此记录表示过去的调整，不是新收入预测或外部付款承诺。请查看当前钱包余额；此记录会保留。" },
    "commission.corrected": { title: "佣金更正记录", body: "历史记录：佣金可用积分曾发生 {points} 积分更正。正数表示过去的补发，负数表示过去的追回；不代表当前余额、新收入或外部付款。请查看当前钱包；此记录会保留。" },
    "withdrawal.order.reviewing": { title: "提现状态记录", body: "历史提现状态：审核中，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。" },
    "withdrawal.order.processing": { title: "提现状态记录", body: "历史提现状态：提现中，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。" },
    "withdrawal.order.paid": { title: "提现状态记录", body: "历史提现状态：已提现，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。" },
    "withdrawal.order.rejected": { title: "提现状态记录", body: "历史提现状态：已驳回，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。" },
    "withdrawal.order.failed": { title: "提现状态记录", body: "历史提现状态：失败，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。" },
    "withdrawal.order.cancelled": { title: "提现状态记录", body: "历史提现状态：已取消，涉及 {points} 积分。此为内部积分记录，不证明外部转账；请查询提现订单的最新状态。" },
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
  const withdrawalEvent = item.event_type.startsWith("withdrawal.order.");
  const commissionEvent = item.event_type === "commission.paid" || item.event_type === "commission.adjusted" || item.event_type === "commission.corrected";
  const rewardEvent = item.event_type.startsWith("reward.order.");
  if ((withdrawalEvent || commissionEvent || rewardEvent) && snapshot === null) {
    throw new RangeError("Withdrawal, commission, and reward notifications require an immutable content snapshot");
  }
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
  const protectedNote = withdrawalEvent || commissionEvent || rewardEvent || (snapshot && (item.event_type === "bet.order.won" || item.event_type === "bet.order.prize_reversed"))
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
