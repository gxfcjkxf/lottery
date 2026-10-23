import { defineComponent, h } from 'vue'
import type { PropType } from 'vue'
import {
  House, LayoutDashboard, Building2, Users, Network, Ticket, SlidersHorizontal,
  CalendarDays, Wallet, ListChecks, Archive, CalendarClock, TrendingUp, Send,
  ArrowLeftRight, Gift, ChartNoAxesCombined, ShieldCheck, ClipboardList, Bell,
  Mail, Scale, Settings, Menu, ChevronDown, ChevronLeft, ChevronRight,
  CircleHelp, Link, ArrowUpRight, Plus, X, Search, RefreshCw, Languages,
  CircleUserRound, Ellipsis, EllipsisVertical, Check, Trash2, CheckCheck, Copy,
} from '@lucide/vue'

const icons = {
  home: House, dashboard: LayoutDashboard, brands: Building2, users: Users,
  network: Network, ticket: Ticket, rules: SlidersHorizontal, calendar: CalendarDays,
  wallet: Wallet, reconcile: ListChecks, archive: Archive, schedule: CalendarClock,
  trend: TrendingUp, send: Send, transfer: ArrowLeftRight, gift: Gift,
  chart: ChartNoAxesCombined, shield: ShieldCheck, audit: ClipboardList,
  bell: Bell, mail: Mail, compliance: Scale, settings: Settings, menu: Menu,
  down: ChevronDown, back: ChevronLeft, next: ChevronRight, help: CircleHelp,
  link: Link, external: ArrowUpRight, plus: Plus, close: X, search: Search,
  refresh: RefreshCw, language: Languages, user: CircleUserRound, more: Ellipsis, moreVertical: EllipsisVertical, check: Check, trash: Trash2, read: CheckCheck, copy: Copy,
} as const
export type SiteIconName = keyof typeof icons

/** One Lucide weight and optical size across all three applications. */
export default defineComponent({
  name: 'SiteIcon',
  props: {
    name: { type: String as PropType<SiteIconName>, required: true },
  },
  setup: props => () => h(icons[props.name], {
    size: 20, strokeWidth: 1.75, class: 'site-icon',
    style: { width: 'var(--site-icon-size, 20px)', height: 'var(--site-icon-size, 20px)', flexShrink: 0 },
    'aria-hidden': 'true', focusable: 'false',
  }),
})
