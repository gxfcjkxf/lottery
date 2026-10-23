import {execFileSync} from 'node:child_process';
import {accessSync} from 'node:fs';
import {resolve} from 'node:path';
import {chromium} from 'playwright';
if(!process.env.TEST_DATABASE_URL)throw new Error('TEST_DATABASE_URL must identify an owned nonproduction test database.');
const browser=chromium.executablePath();accessSync(browser);
execFileSync('go',['test','-race','-tags=drawbrowser','./internal/betting','-run','TestConfiguredBrowserDOM','-count=1'],{cwd:resolve('backend'),stdio:'inherit',env:{...process.env,PLATFORM_DRAW_BROWSER_NODE:process.execPath,PLATFORM_DRAW_BROWSER_SCRIPT:resolve('scripts/render-draw-feed.mjs'),PLATFORM_DRAW_BROWSER_EXECUTABLE:browser}});
