# 审计记录筛选和完整 CSV 导出

授权后台人员可查询品牌审计记录，下载指定时间范围的完整CSV。查看和导出分别授权，成功请求也写入审计；审计未提交、授权变化或超限时不释放数据或文件。读取不修改积分、订单或原审计，不启用支付、资金开关或生产部署。

## 权限和品牌范围

GET `/api/v1/admin/audit`需要audit.view.brand或audit.view.platform。品牌权限只能查询所属品牌；平台查看权限可省略品牌头跨品牌查询。响应仍为`{items:[]}`，旧字段保留，新增brand_id为UUID或null。

GET `/api/v1/admin/audit/export`同时需要对应品牌的查看权和独立audit.export.brand或audit.export.platform。即使拥有平台权限也必须传明确X-Brand-ID，不提供全部品牌一键导出。超级管理员身份不替代权限；拒绝访问沿用access.denied审计。

0071只为既有bootstrap角色追加对应品牌或平台导出权限，自定义角色不扩权。新建bootstrap管理员同样获得独立权限。迁移新增品牌及平台时间倒序索引，不重写审计、模板、余额、账本或订单；生产升级需安排索引创建的锁等待窗口，不自动迁移原开发库。

## 筛选和界限

共享筛选为from、to、action、actor_id、resource_type、resource_id、request_id。时间须为以Z结尾的RFC3339 UTC格式，开始含、结束不含；两者同时提供、开始早于结束、跨度最多31天。查询可不提供时间，导出必须提供。文本精确匹配，最多128 UTF-8字节，无首尾空白或控制字符；人员和资源ID必须是UUID。未知、重复和空值参数拒绝。

查询limit默认100、范围1至200，offset默认0、范围0至100000；按created_at DESC、id DESC排序。查询自身新审计不在本次结果内。逐页查询不是冻结快照；并发新增记录可能移动后续页偏移。后台每页100条，继续读取须明确翻页。完整导出不接受分页、不拼接多页。

完整查询页面序列化记录累计最多4MiB，超限422 AUDIT_QUERY_TOO_LARGE，没有部分items，可缩小范围或分页条数。导出最多10000条，累计记录及最终CSV均有4MiB界限，超限422 AUDIT_EXPORT_TOO_LARGE，没有下载头或截断文件。数据库先限制单条前后JSON传输大小，再限制整个缓冲结果。

## CSV 和导出证据

导出通过主库一条SELECT观察匹配记录并保存语句UTC时刻，不通过只读副本或跨页拼接。十三列为id、brand_id、action、actor_type、actor_id、resource_type、resource_id、reason、request_id、created_at、ip_address、before_json、after_json。日期是原UTC时间；JSON是业务写入时保存的脱敏事实，不推算今天的配置或余额。

CSV使用UTF-8 BOM、RFC4180引号及CRLF。文本首个非空白或控制字符为等号、加号、减号、@，或包含tab、回车、换行时，增加前导单引号防止执行公式。转义只影响副本，不修改原审计。

响应text/csv; charset=utf-8，文件名audit-品牌UUID-v1.csv，带Content-Length、no-store、nosniff，以及X-Audit-Brand-ID、X-Audit-Snapshot-At、X-Audit-Row-Count、X-Audit-SHA256、X-Audit-Format-Version（1）、X-Audit-Export-ID。摘要覆盖BOM及全部字节。audit.export保存筛选、观察时刻、条数、字节数、摘要和格式版本；其UUID是审计证据，不是永久下载地址或异地备份。

主库ACL共享锁下重新认证会话及权限，生成数据前、写审计前及审计等待结束后复核。等待期间会话过期时整体回滚；只在提交后写文件响应。原历史查询辅助函数也补上审计等待后的会话复核，主库授权、可选安全历史副本和提交前不释放数据的边界不变。

## 界面和验收

PC与360px响应式后台支持中英文筛选、逐页查询和前后JSON详情。输入明确解释为UTC，不按设备时区转换。详情对密码、令牌、密钥等键额外显示脱敏；实际审计写入方仍须禁止明文凭据。

导出依据已提交筛选，不使用未提交的输入草稿。浏览器核对品牌、固定文件名、格式、长度、条数、CSV列、排序、时间范围和摘要后才下载并显示证据。失败清理当前回执，不自动重试。账号、品牌、权限或语言变化清空结果及证据，迟到响应不得在新范围下载；不创建资金请求。

专用双端验收使用空owned合成库，正常migrate、seed、create-admin及API启动。真实新增会员、人工充值7、确认并保存前后快照；另创建仅查看管理员，验证独立导出及跨品牌403。页面在新加坡时区输入UTC范围、筛选充值资源、查看详情、下载实际CSV，核对摘要与已提交审计，再切中文重查。原7积分及完整账本保持。每端60秒，零重试或跳过；不代替真机、生产容量或客户人工代码审核。
