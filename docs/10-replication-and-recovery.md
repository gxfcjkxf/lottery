# 本地复制和备份恢复操作手册

本手册供实施与运维人员复现隔离数据库演练：一台主库、两台只读从库，关闭旧主库后手动提升一台从库，并在另一台全新数据库恢复逻辑备份。演练验证合成积分流水、审计和完整表内容，不连接客户生产库，不执行应用自动切换，也不代表生产灾备验收完成。

## 前提和安全边界

- Linux或macOS，Go，以及完整PostgreSQL 17工具目录：initdb、postgres、pg_ctl、pg_basebackup、pg_dump、pg_restore。Windows不运行本演练入口。
- 专用空目录须由操作者通过mktemp创建。已有文件的目录会拒绝使用；每个数据目录都是该目录下的新节点，不清理或覆盖原数据库。
- 默认使用回环端口55531至55534，全部须未占用；可用LOTTERY_RECOVERY_BASE_PORT指定连续四个空闲端口，任何包含5432或55432的节点范围均拒绝。
- 原开发库只作为前后摘要基线，连接必须是显式127.0.0.1地址、不同于四个演练端口。只允许单个sslmode查询参数，不接受host、port、options等重定向。连接和摘要事务都设为只读。
- 工具子进程移除继承的PG连接选项、数据库URL和认证密钥等环境变量，显式连接合成节点。trust认证仅用于回环隔离演练，不能作为生产配置。
- 备份和报告以0600保存，报告路径只新建不覆盖。报告只有行数、摘要和状态，不含账号原文、Cookie、DSN、密钥或SQL结果行。

## 复现命令

从backend运行。将工具目录和原开发库URL改为自己的本地开发环境；报告目录须已存在，文件名须全新：

```sh
export LOTTERY_POSTGRES_BIN='/absolute/postgresql17/bin'
export LOTTERY_RECOVERY_ROOT="$(mktemp -d /tmp/lottery-recovery.XXXXXX)"
export LOTTERY_RECOVERY_CONFIRM_ISOLATED=yes
export LOTTERY_RECOVERY_ORIGINAL_DATABASE_URL='postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable'
export LOTTERY_RECOVERY_REPORT='/absolute/test-output/fresh-recovery.json'
CGO_ENABLED=0 go test -tags recovery -buildvcs=false ./internal/recovery \
  -run '^TestReplicationPromotionAndBackupRestore$' -count=1 -v
```

缺失确认、工具、合法路径或空闲端口会失败，不会静默跳过。普通go test不启动复制节点；核心摘要、校验和回归使用TEST_DATABASE_URL的独立随机schema。

## 演练流程和核验

主库安装当前完整基线`0001_baseline.up.sql`和开发种子，再用积分账本服务创建123积分的合成赠送入账。两台从库经pg_basebackup的流式WAL备份建立，确认pg_stat_replication存在两条streaming连接且写入被25006拒绝。两台启动后，再在主库通过账本服务入账7积分；两台从库均须与主库的全部表摘要一致，不能只比较初始基准备份。pg_basebackup的流式WAL模式和恢复配置由[PostgreSQL 17官方说明](https://www.postgresql.org/docs/17/app-pgbasebackup.html)定义。

对主库public业务schema执行custom格式逻辑备份，另存SHA256及字节数。恢复前必须与原证据匹配，同大小但内容被修改的备份也拒绝。该备份包含业务表、数据、函数与约束，不包含集群角色、外部认证密钥或对象存储。

关闭旧主库并实际核验其进程状态和连接失败后，才手动提升从库1。提升前后的业务摘要应一致；提升后再次通过账本服务入账50积分，余额变为180。从库2重新连接新主库，必须复制到新增流水及全部当前表内容。旧主库保持停止，不重启为另一个可写节点。该步骤仅验证手动操作，不提供自动仲裁、网络隔离或生产防双主机制。生产拓扑还须遵守[PostgreSQL备用服务器机制](https://www.postgresql.org/docs/17/warm-standby.html)并配合实际高可用管理器。

第四台全新节点通过pg_restore的single-transaction及exit-on-error恢复备份。恢复后应回到备份时的130积分，而不是提升后新写入的180积分；这是快照边界，不是时间点恢复。全部表内容摘要、行数和迁移记录须与备份基线完全一致，钱包读取须成功，账本及审计的更新/删除仍被不可变约束拒绝。再次migrate核验已应用SQL校验和。事务式恢复选项见[pg_restore官方说明](https://www.postgresql.org/docs/17/app-pgrestore.html)。

完成或失败时停止本次拥有的节点，保留数据、备份及诊断日志用于核查，不删除原开发库。成功报告在所有节点停止后生成，并核验原开发库public全部表摘要没有变化。若原开发库同时发生业务写入，摘要会变化并使演练不通过；应在安静开发窗口重跑，不忽略差异。

## 函数查找路径

当前完整基线将应用函数的search_path固定为pg_catalog、所属schema、pg_temp。pg_restore使用空会话查找路径时，嵌套配置校验及触发器仍能找到同schema的函数和表，调用方临时同名对象不能优先遮蔽它们。演练核验当前单份基线校验和，不执行旧迁移链升级，也不关闭账本约束。

以后新增数据库函数也须固定到所属schema，兼容随机schema测试和public部署。不能把public硬编码到所有测试函数，不能依赖客户端连接默认search_path。恢复回归会验证空路径下合法代理配置仍通过、临时同名函数不能改变判断，以及应用函数都有固定路径。

## 当前证据与待交付事项

最新[2026年10月10日复制恢复证据](performance/current-recovery-baseline-20261010.json)复跑当前代码：PostgreSQL17.11、120张表、322行、一份完整基线及合成会员账本/审计。两从库只读及全表摘要一致；旧主停止后手动提升、额外入账、另一从库重新连接、逻辑备份恢复及不可变约束均通过。备份1,139,271字节，原开发库前后摘要相同，演练节点已停止；本地追平/提升/恢复分别约0.108/0.110/1.094秒。其他业务表包含结构，不代表完整投注、提现、佣金实际数据经过灾备负载演练。[10月9日报告](performance/current-physical-restore-20261009.json)及[早期报告](performance/s7g-replication-recovery.json)保留为各自执行记录，不覆盖旧证据。

在仓库根目录运行`node scripts/check-recovery-evidence.mjs recovery REPORT.json`，独立核对整表摘要清单及总摘要、恢复前后表/行/单份基线数量、两台从库、旧主隔离、恢复保护及原库未变声明；不接受自动切换、PITR、密钥恢复或生产验收声明。两份当前原始报告和篡改/缺失证据用例进入静态回归。

replica_catchup_seconds从主库新增测试流水到两台从库全部表摘要匹配，包含本地提交和摘要查询成本，不是持续复制延迟指标。promotion_seconds仅为已关闭旧主库后的手动提升及恢复状态确认，不包含检测故障、应用重连或端点切换。restore_seconds包含创建恢复节点、恢复和首次完整摘要检查；这些小数据、同机测量不应设置为生产RTO或RPO。

应用历史查询现已支持下述多读节点路由；资金和当前业务读写仍使用主库。自动主库切换、统一写端点、真实复制延迟告警、WAL归档/PITR、离机加密备份、保留/轮换、角色权限与认证密钥恢复、负载下故障、网络分区和客户环境仍需继续实施及演练。

## 应用历史读路由

服务端仅允许以下六个管理端GET读取从库：`/audit`、`/notification-templates/{key}/history`、`/compliance-policy/history`、`/brand-presentation/history`、`/brand-domains/history`、`/brand-operation/history`，共同前缀为`/api/v1/admin`。其他接口不受读节点配置影响，包括余额、积分流水、报表、提现政策、投注、期次、会话、权限和当前配置。客户端不能通过请求头或查询参数选择节点。

`DATABASE_READ_URLS`是最多八个非空、不重复PostgreSQL连接串的JSON数组；未配置或[]时使用主库。旧单节点变量DATABASE_READ_URL不再支持，非空时启动报错。每台节点的连接预算由部署方显式规划。

读节点必须属于同一物理复制集群、同一数据库、UTF8编码及相同应用schema。配置的选定节点无法通过核验时明确报错，不回退主库或其他节点。使用直接节点地址或整个会话固定到同一节点的连接池；TLS、网络隔离和复制权限由部署方配置。

每次历史查询先在主库验证会话和品牌权限，再获取WAL屏障。按轮转顺序选择一个配置节点，确认其仍在恢复、未暂停且已回放到屏障；核验失败即报错。核验成功后在同一物理连接上建立只读repeatable read快照。当前配置、资金、权限和审计仍使用主库。

选定节点的连接与探测限100毫秒，数据查询限两秒。失败或超时丢弃结果并返回错误，不改用主库、不重新选择节点。主库探测使用保存点，失败不会污染调用方事务；需要的监控权限须由运维显式配置。

数据返回前再次验证主库授权、追加并提交主库查询审计。主库授权、审计或提交失败时不释放历史数据；从库不能承担主库失联后的授权或写审计。成功响应提供`X-Read-Source: primary|replica`、固定词汇的`X-Read-Reason`，从库响应还包含从1开始的`X-Read-Replica`配置索引；不暴露DSN、节点地址、密钥或内部异常。历史查询中的品牌运行、展示、域名审计分别使用`brand_operation.history`、`brand_presentation.history`、`brand_domains.history`，当前读取仍使用原`*.view`。

WAL屏障确保读取包含采样位置之前已提交并回放的历史，不代表响应时刻与主库完全同步，也不提供自动故障切换或分布式事务。

## 读路由复现

使用本手册相同的PostgreSQL工具、隔离确认、原库只读地址和四个空闲端口；为每轮新建空目录和全新的报告路径。从backend执行：

```sh
export LOTTERY_RECOVERY_ROOT="$(mktemp -d /tmp/lottery-history-routing.XXXXXX)"
export LOTTERY_RECOVERY_REPORT='/absolute/test-output/fresh-history-routing.json'
CGO_ENABLED=0 go test -tags recovery -buildvcs=false ./internal/recovery \
  -run '^TestPhysicalHistoryReadRouting$' -count=1 -v
```

演练只启动拥有的一主两从及一个无关集群；结束时停止这些节点并保留数据、日志和报告。原开发库仅作前后只读摘要，不暂停或修改原开发服务。固定字段报告不保存登录令牌、请求正文、数据库连接串或客户资料；真实高可用和生产容量验收仍独立进行。

[当前实体读路由证据](performance/current-physical-history-20261009.json)已验证两个从库轮转、6条真实品牌HTTP历史接口、暂停/离线/错误集群/查询超时拒绝、权限撤销403、会话撤销401及主库审计失败不返回历史数据。原开发库摘要相同，节点已停止。初轮夹具只等标记行可见，仍落后于新采样WAL；现先等指定从库回放到实际位置，再在有界准备阶段等待新屏障就绪。准备阶段的独立测试请求不是生产自动重试，生产路由每次选定一个节点，失败即报错，不换节点或回退主库。[旧实体读路由报告](performance/s7m-history-read-routing.json)仅作开发记录，不用于证明当前无回退语义。

[10月10日复跑报告](performance/current-physical-history-20261010.json)再次通过上述六条真实接口及全部拒绝/权限检查，两个从库均被实际选中，原库摘要未变，节点停止。`node scripts/check-recovery-evidence.mjs history REPORT.json`核对这组明确的成功/拒绝证据与节点索引，不把历史读路由当作应用写端点自动切换。当前复跑总耗时19.87秒，不是生产查询延迟或恢复SLO。

## 生产恢复前必须确认

运维人员须核对已授权的目标集群、备份来源与完整性、恢复时间点及数据损失范围，停止或隔离旧主库并控制应用写入，确认高可用仲裁与端点策略。备份文件、角色权限、认证密钥和外部资源应分别从受控存储恢复；不能在仓库中保存生产密钥。恢复后先核对账本、注单、期次、幂等回执和持久任务，再按已确认策略恢复API及worker。

在客户环境中完成实际恢复及故障演练、达到确认的RTO/RPO、通过人工审核以前，不应将生产灾备或整个项目标记完成。
