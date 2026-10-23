export const timeZoneCatalog = {
  "en": {
    "Utc": "UTC",
    "Shanghai": "Shanghai",
    "Singapore": "Singapore",
    "Tokyo": "Tokyo",
    "Seoul": "Seoul",
    "Kolkata": "Kolkata",
    "Dubai": "Dubai",
    "Jakarta": "Jakarta",
    "Bangkok": "Bangkok",
    "HongKong": "Hong Kong",
    "Taipei": "Taipei",
    "London": "London",
    "Berlin": "Berlin",
    "Paris": "Paris",
    "Moscow": "Moscow",
    "NewYork": "New York",
    "Chicago": "Chicago",
    "Denver": "Denver",
    "LosAngeles": "Los Angeles",
    "Toronto": "Toronto",
    "SaoPaulo": "São Paulo",
    "Sydney": "Sydney",
    "Auckland": "Auckland",
    "choose": "Select a time zone"
  },
  "zh-CN": {
    "Utc": "协调世界时",
    "Shanghai": "上海",
    "Singapore": "新加坡",
    "Tokyo": "东京",
    "Seoul": "首尔",
    "Kolkata": "加尔各答",
    "Dubai": "迪拜",
    "Jakarta": "雅加达",
    "Bangkok": "曼谷",
    "HongKong": "香港",
    "Taipei": "台北",
    "London": "伦敦",
    "Berlin": "柏林",
    "Paris": "巴黎",
    "Moscow": "莫斯科",
    "NewYork": "纽约",
    "Chicago": "芝加哥",
    "Denver": "丹佛",
    "LosAngeles": "洛杉矶",
    "Toronto": "多伦多",
    "SaoPaulo": "圣保罗",
    "Sydney": "悉尼",
    "Auckland": "奥克兰",
    "choose": "请选择时区"
  }
} as const

export const timeZoneOptions = [
  [
    "UTC",
    "Utc"
  ],
  [
    "Asia/Shanghai",
    "Shanghai"
  ],
  [
    "Asia/Singapore",
    "Singapore"
  ],
  [
    "Asia/Tokyo",
    "Tokyo"
  ],
  [
    "Asia/Seoul",
    "Seoul"
  ],
  [
    "Asia/Kolkata",
    "Kolkata"
  ],
  [
    "Asia/Dubai",
    "Dubai"
  ],
  [
    "Asia/Jakarta",
    "Jakarta"
  ],
  [
    "Asia/Bangkok",
    "Bangkok"
  ],
  [
    "Asia/Hong_Kong",
    "HongKong"
  ],
  [
    "Asia/Taipei",
    "Taipei"
  ],
  [
    "Europe/London",
    "London"
  ],
  [
    "Europe/Berlin",
    "Berlin"
  ],
  [
    "Europe/Paris",
    "Paris"
  ],
  [
    "Europe/Moscow",
    "Moscow"
  ],
  [
    "America/New_York",
    "NewYork"
  ],
  [
    "America/Chicago",
    "Chicago"
  ],
  [
    "America/Denver",
    "Denver"
  ],
  [
    "America/Los_Angeles",
    "LosAngeles"
  ],
  [
    "America/Toronto",
    "Toronto"
  ],
  [
    "America/Sao_Paulo",
    "SaoPaulo"
  ],
  [
    "Australia/Sydney",
    "Sydney"
  ],
  [
    "Pacific/Auckland",
    "Auckland"
  ]
] as const

export function timezoneOffset(timezone: string): string {
  const part = new Intl.DateTimeFormat('en-US', { timeZone: timezone, timeZoneName: 'shortOffset' })
    .formatToParts(new Date()).find(value => value.type === 'timeZoneName')?.value
  if (!part) throw new Error(`Missing UTC offset for ${timezone}`)
  if (part === 'GMT') return '+0'
  const match = /^GMT([+-])(\d{1,2})(?::(\d{2}))?$/.exec(part)
  if (!match) throw new Error(`Unsupported UTC offset format for ${timezone}: ${part}`)
  return `${match[1]}${match[2]}${match[3] === '00' || !match[3] ? '' : `:${match[3]}`}`
}
