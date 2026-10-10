# WebKit 功能回归交接

这一专项补充非Chromium引擎的真实页面证据，运行用户和品牌后台功能，不验收PWA离线缓存。当前在macOS使用锁定Playwright 1.62.1安装的WebKit 26.5，通过1440px桌面和360px手机模拟视口；不安装或覆盖系统Safari。

Playwright的WebKit来自WebKit主干，不是品牌版Safari，不能据此宣称真实iPhone/iOS或Safari版本已验收。[官方浏览器说明](https://playwright.dev/docs/browsers#webkit)明确区分二者。该工具的服务工作线程支持仅面向Chromium，本专项明确设置serviceWorkers为block；不把功能回归当作WebKit PWA安装或缓存验证。[服务工作线程说明](https://playwright.dev/docs/service-workers)

## 当前覆盖

| 场景 | 真实验证 |
| --- | --- |
| 用户账号 | 注册、Cookie恢复、首次资料补充、站内通知与原键读回执、退出 |
| 运营新增会员 | 后台新增后由本人接受当前品牌条款 |
| 后台角色账号 | 创建、重载持久化、仅指定权限、双端布局 |
| 风控政策 | 七组控件、账号关联风险开关、丢失回执的原键恢复、真实拒绝、恢复原政策 |

三个文件auth.spec.ts、management.spec.ts、compliance.spec.ts在两个视口运行，共八项；串行、无重试、不复用已有前端服务。测试使用正式API、worker和独立UTF8合成库，不模拟业务成功响应。它不覆盖全部投注/开奖/提现/佣金、全部权限组合、其他OS或真实外部支付。

## 运行

先安装锁定包对应的引擎：

```sh
pnpm exec playwright install webkit
```

准备独立合成数据库，按当前单份基线migrate/seed，分别启动API和worker（127.0.0.1:8080）。显式引导Aurora和Harbor普通品牌账号，设置三对变量：TEST_ADMIN_USERNAME/PASSWORD、TEST_HARBOR_ADMIN_USERNAME/PASSWORD、TEST_COMPLIANCE_ADMIN_USERNAME/PASSWORD。不使用客户或生产凭据，不清除认证限流。

设置PLAYWRIGHT_WEBKIT_EXECUTABLE_PATH为已安装引擎的绝对可执行路径，然后：

```sh
pnpm test:webkit
```

路径缺失、不存在或凭据缺失立即报错，不改用Chrome、不跳过。配置自动启动三套前端5173/5174/5175；不要让已有进程占用这些端口。开发机不自动安装系统库或调整密码安全参数。

## 独立 CI 运行

webkit-functional任务使用GitHub Actions Linux运行器与独立lottery_webkit_test数据库。先按锁定包安装WebKit及Linux依赖，实际启动引擎并验证页面，再把该引擎的绝对路径提供给配置。使用正式单份数据库基线、CLI引导三个品牌账号及API/worker，不共享Chromium任务的服务或业务数据。

任务运行同一八项功能测试，串行、无测试重试、无失败放行。失败时保留浏览器trace和专项服务日志；引擎安装、启动或业务测试失败均使任务失败。当前本机功能结果、CI配置检查与实际远程任务成功分别记录；不能把配置校验或Chromium结果当作Linux WebKit成功证据，也不能把Linux结果当作Safari/iPhone真机验收。
