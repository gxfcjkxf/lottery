# 用户端列表核查（2026-11-02）

按统一设计系统核查实际列表入口、筛选、分页、加载、成功空结果及失败恢复。列表复用 `user-record-list` 单一外框、`ui-filter-bar`、`TablePagination` 和共享空状态；用户端顶栏继续提供唯一页面标题及原有刷新入口。

| 列表 | 页面 / 组件 | 处理与验证 |
| --- | --- | --- |
| 积分流水 | `/wallet/ledger` · `WalletSummary.vue` | 每页 50 条；以真实总数决定下一页；读取成功后提交页码，失败保留记录及页码 |
| 投注记录 | `/orders` · `BettingHistory.vue` / `OrderHistoryLayout.vue` | 每页 20 条，游标仅允许访问已知页面；刷新及失败保留已展示记录；彩种及账号切换仍清除旧范围 |
| 充值记录 | `/recharge` · `RechargePanel.vue` | 状态、查询、清空与记录共用面板；分页读取已应用条件；查询成功回到第一页，失败不清空原页 |
| 提现记录 | `/withdraw` · `WithdrawalPanel.vue` | 接入已有每页 20 条接口分页；保留申请、余额及资格保护，未知申请仍按原键原体重试 |
| 推荐列表 / 推荐码 | `/invites` · `JoinCodesPanel.vue` | 页签、列表及分页共用面板；每页 20 条；成功空结果保留分页，失败与空结果分离 |
| 通知 | `/notifications` · `NotificationsPanel.vue` | 桌面每页 20 条并保留成功空结果分页；移动端按规范保留按需加载；加载、空结果、失败及重试分别呈现 |
| 开奖结果 | `/results` 及投注页结果区 · `DrawArchive.vue` | 保留年份 / 日期与号码业务布局，每页 50 条；筛选、记录及分页处于同一容器；失败保留已应用范围及记录，翻页保持滚动位置 |

首页彩种入口、号码 / 玩法配置、钱包四种固定来源是导航或有限配置集合，不添加记录分页。单条记录详情及提现操作历史保留各自业务结构。

验证覆盖中英文、Android Pixel 7 和 iPhone 13 横竖屏、Chromium / WebKit 的 320、620、900、1440 CSS px 重排。使用合成接口验证零条、多页、筛选、清空、失败恢复与详情；提现写入回归继续核对未知结果的原请求重试，不写入共享数据库。

`BettingHistoryTable.vue` 是所属注单页的明细表；`ConfiguredBetReviewTable.vue` 是单次投注确认的固定明细快照。两者保留表格语义，不独立添加记录分页。新增记录集合会由 Vue AST 扫描要求登记，不能靠不使用分页组件而漏出核查范围。

真实页面验收由 `user-lists.spec.ts`、`user-list-review.spec.ts`、`referral-tabs.spec.ts`、`notification-bet.spec.ts` 和 `results-fixed.spec.ts` 执行。均直接访问真实路由，加载用户端 `main.ts`、路由守卫、应用壳和样式；共用 `real-page-review.ts` 检查外框、筛选、表头及分页实际结构。空结果之外，覆盖充值／提现／流水多页、查询及失败保留原页，推荐列表和推荐码、通知注单详情，以及有记录的开奖滚动和固定操作区。设备预设保持横竖屏原始视口；只有明确的 reflow 项目调整宽度，检查后恢复其初始视口。
