# 自动日月归档核心交接

自动归档沿用[不可变快照与版本合同](26-report-archive-core.md)：按品牌时区划分日/月，财务按实际入账时间统计，保存一次数据库观察，不封账、不改变积分，也不覆盖旧档。0067已接入内部配置、日历发现、持久任务、失败人工重试和独立worker。所有品牌的日/月开关初始关闭；当前配置只读、任务查询/重试HTTP及双端页面已接入，见[28号管理合同](28-report-archive-task-management.md)。配置写入和启用入口仍待实现。

首次启用的业务起点尚待确认：从启用当天/当月开始，还是允许配置历史回补。当前内部服务只接受明确保存的起始周期，不自动替用户选择，也不通过迁移回补历史。人工归档原四条接口与任务管理新四条接口分开，不提供公开启用或回补操作。

## 配置与权限

brand_report_archive_policies保存品牌当前配置，字段为brand_id、version、daily_enabled、monthly_enabled、daily_start_period、monthly_start_period、timezone、audit_log_id和updated_at。版本从1开始，初始开关false、起点和审计null，时区来自品牌。新建品牌同样初始化关闭。

UpdateAutomaticPolicyTx输入当前version、两个显式开关、两个可空起点和reason。启用相应类型必须有规范起点，采用该次变更时品牌具名时区；修改须精确匹配版本及管理员审计，每次保存下一版本，并在report_archive_policy_revisions保留不可变原配置。配置保存不生成归档、不操作账本。日/月可独立启停，暂停品牌允许报表，停用品牌拒绝配置和自动执行。

查看沿用report_archive.view.brand或显式report_archive.view.platform。修改必须同时具备品牌查看和report_archive_policy.write.brand；重试必须同时具备品牌查看和report_archive_task.retry.brand。超级管理员只读。0067只为品牌引导角色追加两个新权限，不扩大平台或自定义角色；CLI初始化遵守同样范围。内部读取需要调用方授权，不能直接作为无审计HTTP入口。

## 周期发现

每个启用的配置版本保存独立日/月游标，从该版本明确保存的起点开始。DiscoverAutomatic按真实民用日/月推进，只登记to已到的范围，不使用固定24小时或30天。每次最多登记100个任务，worker每分钟尝试最多20个，30秒上下文预算。游标与新任务及系统审计同事务提交，技术失败回滚该批，已提交其他批仍保留。

同品牌、类型、周期唯一，不因配置版本或时区变化再次登记。同一已有任务保留原policy_version、timezone、from和to，新配置不改写旧任务。游标推进需数据库证明每个真实已到范围已经登记，不能悄悄丢掉中间日/月。整天不存在的民用日期不造空归档；前一天仍保留，结束边界取名义次日之后第一个真实时刻。阿皮亚2011-12-30被跳过，但2011-12-29正常归档。夏令时、重复午夜和午夜缺口继续沿用真实边界算法。

## 持久任务与执行

AutomaticTask内部DTO字段为id、brand_id、policy_version、window、state、version、attempt_count、archive_id、last_error_code、creation_audit_log_id、last_audit_log_id、created_at和updated_at。window包含kind、period_key、timezone、from和to。任务版本与尝试计数为安全整数；分页最多100条，总数为精确十进制字符串。游标不是管理任务DTO的一部分。

任务最初pending、version为1、attempt_count为0。ProcessAutomatic锁定一个任务并复核当前品牌及对应日/月开关，以保存的原时区和绝对区间执行：

- completed：不存在旧档，保存系统首版，并在同一事务完成任务和审计。
- skipped：该周期已有真实完整归档，记录其ID，不覆盖、不自动追加观察版本。
- failed：技术或完整性失败回滚捕获事务，在另一个事务保存失败结果，错误码仅ARCHIVE_FAILED，不公开底层SQL或资料。设施恢复不会自动重试failed。

每次从pending终结时，任务版本和attempt_count各加1。RetryAutomaticTaskTx只允许授权管理员对匹配版本的failed填写原因，将原任务恢复pending；版本加1、尝试数不变，原配置和区间不变。重试仍须当前品牌与相应开关允许。busy锁不会被当作证据失败。数据库或审计完全不可用时，失败记录也可能无法保存；方法返回错误，不能宣称该次已经记为failed。

关闭开关只暂停新发现及pending执行，不删旧任务、不撤销已有归档。重新开启允许处理旧pending，failed仍需人工重试。已存在档案需要重新观察时，由运营人员使用原人工追加接口，不由系统周期轮询无限追加。worker每秒执行最多20步，10秒上下文预算，在独立循环运行，不阻塞一秒期次时钟。

## 原子性与系统来源

worker与人工创建使用同一schema内的品牌、类型、周期系列锁，不同测试或部署schema互不误锁。任务采用FOR UPDATE SKIP LOCKED及品牌/当前配置共享锁，多实例可以共同执行。捕获与插入共用一个SQL语句和只读STABLE来源校验，正常资金提交不被全品牌钱包锁阻塞。

系统归档created_by为null，额外automation对象恰好包含task_id及policy_version；人工记录created_by仍为原管理员UUID，并省略automation。原人工记录的十二字段结构不变，系统记录增加一个字段；snapshot格式仍为1，payload原字节与SHA-256算法不变，系统来源不混入资金统计内容。管理SDK、OpenAPI和现有详情页面已支持两种来源，人工创建回执不能被系统记录冒充。

数据库校验系统审计、真实pending任务、保存配置、当前开关、完整统计与摘要。延迟关联守卫要求归档提交时存在精确completed任务及反向archive_id，不允许半份系统档案。UPDATE、DELETE、TRUNCATE归档拒绝规则保留。0067保留原手动守卫OID，只增加自动来源分支、schema锁命名空间和跳过日期的结束边界修正；原版本追加仍使用已保存区间，不重新解释旧档。

## 升级与交付边界

先执行正常migrate，再协调API、worker和管理前端版本，程序不会自动迁移。0067增加四张任务/配置表及归档两个可空关联字段，只放宽系统归档的created_by，不修改旧归档、钱包、账本、投注、财务操作时间或审计历史。十个新增函数固定pg_catalog、应用schema、pg_temp查找路径。默认无游标、无任务、无历史补造档；重复迁移不重复授权。

真实PostgreSQL回归覆盖日/月、并发发现和执行、启停保留队列、停用/暂停品牌、人工旧档跳过、实际13积分观察、失败回滚与人工重试、半份完成拒绝、后续人工追加和日历边界一致。0066到0067升级逐行摘要保留真实入账、人工旧档、原创建者及原payload/SHA；精确核对两个品牌权限增量及新品牌默认关闭。

内部核心及任务管理已完成，运营人员可以查询配置/任务并显式重试failed，但不能在页面启用自动归档。首次启用口径与配置写入/启用页面继续确认及实施。生产容量、客户人工审核、真机和外部文件保留/财务模板验收另行完成；默认不自动删除保持不变。
