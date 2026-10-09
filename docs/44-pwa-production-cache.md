# 三端 PWA 与生产缓存交接

当前范围是生产构建的安装元数据、图标和公开静态资源缓存，不是离线业务系统。不改变账号权限、积分或订单，不接入支付，不上线应用市场。

## 应用与资源

| 项目 | 安装名称 | 独立缓存名 |
| --- | --- | --- |
| user-web | Lottery user portal | lottery-user-static-v1 |
| admin-web | Brand administration | lottery-brand-static-v1 |
| platform-web | Platform administration | lottery-platform-static-v1 |

三端独立部署，manifest 的 id、scope、start_url 均为 `/`，display 为 standalone。各有 192/512px PNG 安装图标及 180px Apple touch icon；原生 SVG 是图标来源。安装名称和图标是项目通用资源，不随品牌展示配置动态变更。生产必须使用安全上下文；本地验收使用 loopback，不构成生产 TLS 验收。

重新生成九个已知 PNG 文件：

```sh
node scripts/build-pwa-icons.mjs /absolute/path/to/chromium
```

也可显式设置 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH。脚本要求绝对路径，不自动安装或切换浏览器；仅从三端已有 favicon.svg 渲染指定图标。

## Worker 缓存规则

仅生产构建注册 `/sw.js`，updateViaCache 为 none。Worker 只处理同源、无查询串、非导航的 GET，路径须为 `/assets/` 下带至少八位构建哈希的 JS、CSS、字体或图片。仅 HTTP200、basic 响应、扩展名匹配的 MIME 且不含 private/no-store 才保存；已保存资源从本应用缓存读取。

HTML、登录页、manifest、API、钱包、订单和写请求不进入缓存。网络错误不替换为旧业务数据或假成功，不排队、不自动重放投注或提现。只访问本应用缓存，不读取或删除其他缓存；没有旧版本缓存兼容与清理分支。冷启动离线不能加载 HTML，不宣称完整应用可离线使用。

## 验证与 CI

```sh
PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/absolute/path/to/chromium pnpm test:pwa
pnpm test:static
```

test:pwa 先构建全部前端，再启动三个生产预览：127.0.0.1:15173、15174、15175。专用 vite.pwa-preview.config.mjs 明确没有 API 代理，不连接数据库或已有体验服务。Playwright 不复用已占用服务，每个服务独立就绪检查，完成后释放端口；六个项目分别覆盖三端1440px和360px，无重试。

测试验证真实 manifest、PNG 解码尺寸、生产 worker 控制、实际 CacheStorage 键、静态资源原字节及断网后读取。离线资源读取复用已保存资源的 Request，经 fetch 和 worker 返回，不直接读取缓存正文；不据此宣称任意新请求头组合均可命中。API 响应是明确标记的合成路由：两次 GET 返回不同 nonce、POST 独立响应，撤去路由后断网 GET/POST 均失败且缓存不增加。它不是实际钱包、身份或资金交易验证。

静态测试另外覆盖非哈希路径、查询串、导航、跨源、错误 MIME、private/no-store、404 与非 basic 响应不缓存。CI 的 pwa-production-cache 使用同一根命令，不启动 PostgreSQL。

## 尚待验收

实际投注/提现断网与恢复的完整业务验收、生产域名/TLS、安装提示以及 iOS/Android/HarmonyOS 真机安装和交互仍待独立验证。Chrome 的模拟视口不能替代真机安装测试，检查入口见 [Chrome PWA 调试文档](https://developer.chrome.com/docs/devtools/progressive-web-apps)。本阶段不关闭完整 UI、安全或生产验收项。
