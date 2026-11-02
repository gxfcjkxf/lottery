# 官方香港六合彩攪珠日期同步

2026-10-25。本功能只同步官方日期日历，不修改现有开奖结果来源、期号、开奖时分、自动建期或投注流程。官方日历不含未来期号和具体开奖时分，系统不自行推算。

## 数据源与执行

- 页面：https://bet.hkjc.com/ch/marksix/fixtures
- 配置：https://bet.hkjc.com/Config/GlobalConfig.js
- 日期接口：https://consvc.hkjc.com/JCBW/api/graph

采集器读取官网公开配置中的 `PUBLISH.SITECORE_APIKEY`，然后执行页面使用的 `MarksixFixtures` GraphQL 查询，读取 `Schedule` 下的年份、月份、`NormalDrawDates`、`SnowballDrawDates` 和 `PresellDrawDates`。不使用浏览器登录或预览环境，不把公开 CMS key 保存到数据库或日志。

任务随现有 `platform worker` 运行，每分钟检查数据库中的到期时间，正常每天香港时间 04:00 采集一次。首次启动立即采集，停机错过执行时间后启动补采；任务进度保存在数据库中，重启不会重复采集。多 worker 通过事务级 advisory lock 串行执行，不阻塞业务期次时钟。

配置和日期请求各有 15 秒超时，总任务有 40 秒超时，不跟随重定向。HTTP、GraphQL、缺失字段、非法日期等错误明确失败，不切换其他来源。来源失败后保留上一份日历，记录错误码，一小时后重试；成功后恢复每日计划。数据库写入失败整体回滚，在后续分钟检查中重试并输出错误日志。

## 手动立即同步

总后台日历面板的「立即同步」需要 `platform_game.write.platform`；只读账号不展示按钮，服务端独立拒绝无权限请求。`POST /api/v1/platform/platform-games/official-calendar/sync` 携带空对象 `{}` 与 `Idempotency-Key`，不接受来源地址或查询参数。手动请求不受下次计划时间限制，但与每日 worker 共用同一事务锁；已有采集返回 `409 HKJC_CALENDAR_BUSY`，不会并发访问来源。

采集完成返回 HTTP 200 和 `{status:"synced"|"failed",months:number}`，页面按实际 `status` 显示成功或失败，不能仅凭 HTTP 200 提示成功。成功后重新读取本地日历与同步信息；来源失败保留日历，并更新一小时后重试计划。操作结果、同步元数据、审计和幂等回执在同一事务提交；同键重放不再次采集，且重新校验当前会话和权限。完成的失败结果用新请求再次尝试；网络或服务端异常未确认结果时，保留原键，通过「重试本次同步」恢复。

采集接口的处理期限为 40 秒，HTTP 服务写入期限为 45 秒以容纳官网两次请求；其他管理接口保留原有 10 秒处理期限。

## 本地数据

`hkjc_calendar_months` 按月份保存全部攪珠日、金多宝日期、预售日期和同步时间。全部攪珠日取普通日期与金多宝日期的并集；官网部分月份的金多宝日期不出现在普通日期列表中。每次成功原子替换官方返回的月份；当前及未来月份若被撤下，删除对应本地月份；已保存且不再返回的历史月份保留。公布月份内的日期调整会替换旧日期，不累积失效日期。

`hkjc_calendar_sync` 保存最近尝试、最近成功、下次执行和安全错误码。该日历为平台级公开数据，不归属品牌。总后台可手动触发同一官方采集器，不提供修改日期或配置任意来源的接口。

总后台「开奖」页顶部直接展示未来日期，按月分组并标记金多宝，展示最近成功同步时间、下次计划时间及同步状态。日期本身保持香港日历日期，时间戳按平台展示设置格式化。页面提供「重新读取」，只读取本地数据，不触发外网采集。

只读接口 `GET /api/v1/platform/platform-games/official-calendar` 需要精确权限 `platform_game.view.platform`，成功读取写入审计；品牌后台无此读取入口。响应包含 `dates: [{date,is_snowball}]`、`status`、`last_success_at`、`last_attempt_at`、`next_attempt_at`；未记录时间为 `null`。返回香港当天及之后的日期，不返回来源配置或 CMS key。状态为 `pending`（等待首次成功）、`synced`（已同步）、`failed`（最近来源采集失败）、`overdue`（已有成功记录，但超过下次计划时间五分钟）。失败时仍显示上次成功日历；接口读取失败时展示明确的重试提示，不把旧页面当成最新状态。

查询未来攪珠日：

```sql
SELECT d.draw_date,
       d.draw_date = ANY(m.snowball_dates) AS is_snowball,
       m.synced_at
FROM hkjc_calendar_months m
CROSS JOIN LATERAL unnest(m.draw_dates) AS d(draw_date)
WHERE d.draw_date >= (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date
ORDER BY d.draw_date;

SELECT last_attempt_at, last_success_at, last_error_code,
       next_attempt_at AT TIME ZONE 'Asia/Hong_Kong' AS next_attempt_hong_kong
FROM hkjc_calendar_sync;
```

新表合并到 `0001_baseline.up.sql`。已有开发库须先备份并恢复副本，验证增补后的结构等于新基线、原业务表数据完整，再显式同步迁移记录。不能直接运行旧库的 `migrate` 或仅更新 checksum。

## 验证

`backend/internal/hkjccalendar` 包含官网日期样本、请求协议与错误响应测试，以及独立测试库上的每日去重、并发 worker、日期替换、失败保留旧数据、写入回滚和业务表不创建数据验证。
