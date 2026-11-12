import type { LocaleCode } from './brand'

export type NotificationCopy = { title: string; body: string }
export type NotificationTranslations = Record<LocaleCode, NotificationCopy>

/** Version 1 of system notification translations. Database records store references and event parameters. */
export const notificationCatalog = {
  "member.joined": {
    "en": {
      "title": "Welcome",
      "body": "Your membership is ready. Welcome aboard."
    },
    "zh-CN": {
      "title": "欢迎",
      "body": "您的会员账号已准备就绪，欢迎加入。"
    }
  },
  "recharge.confirmed": {
    "en": {
      "title": "Recharge confirmed",
      "body": "Recharge confirmed: {points} points."
    },
    "zh-CN": {
      "title": "充值已确认",
      "body": "充值已确认：{points} 积分。"
    }
  },
  "reward.order.granted": {
    "en": {
      "title": "Reward credited",
      "body": "{points} points credited to gift balance."
    },
    "zh-CN": {
      "title": "奖励发放",
      "body": "赠送可用积分已入账 {points}。"
    }
  },
  "reward.order.revoked": {
    "en": {
      "title": "Reward reversed",
      "body": "{points} points reversed from gift balance."
    },
    "zh-CN": {
      "title": "奖励撤销",
      "body": "赠送可用积分已撤销 {points}。"
    }
  },
  "bet.order.placed": {
    "en": {
      "title": "Order submitted",
      "body": "Order submitted: {points} points."
    },
    "zh-CN": {
      "title": "注单已提交",
      "body": "注单已提交，涉及 {points} 积分。"
    }
  },
  "bet.order.cancelled": {
    "en": {
      "title": "Order cancelled",
      "body": "Cancelled. {points} points were returned to your original balance."
    },
    "zh-CN": {
      "title": "注单已取消",
      "body": "注单已取消，{points} 积分已原路退回。"
    }
  },
  "bet.order.judged_cancelled": {
    "en": {
      "title": "Order cancelled after review",
      "body": "Cancelled after review. {points} points were returned to your original balance."
    },
    "zh-CN": {
      "title": "注单已判定取消",
      "body": "注单经判定已取消，{points} 积分已原路退回。"
    }
  },
  "bet.order.abnormal": {
    "en": {
      "title": "Order needs review",
      "body": "Your order needs manual review. Points involved: {points}."
    },
    "zh-CN": {
      "title": "注单待人工处理",
      "body": "您的注单需要人工处理，涉及积分：{points}。"
    }
  },
  "bet.order.won": {
    "en": {
      "title": "Prize credited",
      "body": "{points} prize points credited."
    },
    "zh-CN": {
      "title": "派奖入账",
      "body": "奖金已入账 {points} 积分。"
    }
  },
  "bet.order.prize_reversed": {
    "en": {
      "title": "Prize reversed",
      "body": "{points} prize points reversed."
    },
    "zh-CN": {
      "title": "奖金冲正",
      "body": "奖金已冲回 {points} 积分。"
    }
  },
  "commission.paid": {
    "en": {
      "title": "Commission credited",
      "body": "{points} points credited to commission balance."
    },
    "zh-CN": {
      "title": "佣金入账",
      "body": "佣金已入账 {points} 积分。"
    }
  },
  "commission.corrected": {
    "en": {
      "title": "Commission corrected",
      "body": "Commission corrected by {points} points."
    },
    "zh-CN": {
      "title": "佣金更正",
      "body": "佣金已更正 {points} 积分。"
    }
  },
  "draw.result.published": {
    "en": {
      "title": "Draw result published",
      "body": "Draw result published: {resource_id}."
    },
    "zh-CN": {
      "title": "开奖结果已公布",
      "body": "开奖结果已公布：{resource_id}。"
    }
  },
  "draw.result.corrected": {
    "en": {
      "title": "Draw result corrected",
      "body": "Draw result corrected: {resource_id}."
    },
    "zh-CN": {
      "title": "开奖结果已更正",
      "body": "开奖结果已更正：{resource_id}。"
    }
  },
  "commission.adjusted": {
    "en": {
      "title": "Commission adjusted",
      "body": "Commission adjusted by {points} points."
    },
    "zh-CN": {
      "title": "佣金调整",
      "body": "佣金已调整 {points} 积分。"
    }
  },
  "withdrawal.order.reviewing": {
    "en": {
      "title": "Withdrawal under review",
      "body": "Withdrawal under review: {points} points."
    },
    "zh-CN": {
      "title": "提现审核中",
      "body": "提现审核中，{points} 积分。"
    }
  },
  "withdrawal.order.processing": {
    "en": {
      "title": "Withdrawal processing",
      "body": "Withdrawal processing: {points} points."
    },
    "zh-CN": {
      "title": "提现中",
      "body": "提现中，{points} 积分。"
    }
  },
  "withdrawal.order.paid": {
    "en": {
      "title": "Withdrawal paid",
      "body": "Withdrawal paid: {points} points."
    },
    "zh-CN": {
      "title": "已提现",
      "body": "已提现，{points} 积分。"
    }
  },
  "withdrawal.order.rejected": {
    "en": {
      "title": "Withdrawal rejected",
      "body": "Withdrawal rejected: {points} points."
    },
    "zh-CN": {
      "title": "提现已驳回",
      "body": "提现已驳回，{points} 积分。"
    }
  },
  "withdrawal.order.failed": {
    "en": {
      "title": "Withdrawal failed",
      "body": "Withdrawal failed: {points} points."
    },
    "zh-CN": {
      "title": "提现失败",
      "body": "提现失败，{points} 积分。"
    }
  },
  "withdrawal.order.cancelled": {
    "en": {
      "title": "Withdrawal cancelled",
      "body": "Withdrawal cancelled: {points} points."
    },
    "zh-CN": {
      "title": "提现已取消",
      "body": "提现已取消，{points} 积分。"
    }
  }
} as const satisfies Record<string, NotificationTranslations>

export function resolveNotificationContent(value: unknown, eventKey: string): NotificationTranslations {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new RangeError('Invalid notification content')
  const source = value as Record<string, unknown>
  if ('i18n_key' in source || 'i18n_version' in source) {
    if (Object.keys(source).length !== 2 || source.i18n_key !== eventKey || source.i18n_version !== 1 || !Object.hasOwn(notificationCatalog, eventKey)) {
      throw new RangeError('Unsupported notification translation reference')
    }
    const copy = notificationCatalog[eventKey as keyof typeof notificationCatalog]
    return { en: { ...copy.en }, 'zh-CN': { ...copy['zh-CN'] } }
  }
  if (Object.keys(source).length !== 2 || !Object.hasOwn(source, 'en') || !Object.hasOwn(source, 'zh-CN')) throw new RangeError('Invalid custom notification translations')
  for (const locale of ['en', 'zh-CN'] as const) {
    const copy = source[locale] as Record<string, unknown> | null
    if (!copy || typeof copy !== 'object' || Array.isArray(copy) || Object.keys(copy).length !== 2 || typeof copy.title !== 'string' || typeof copy.body !== 'string') {
      throw new RangeError('Invalid custom notification copy')
    }
  }
  return value as NotificationTranslations
}
