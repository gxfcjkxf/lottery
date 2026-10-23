// Trusted renderer: source definitions supply selectors and a timeout, never code.
import {chromium} from 'playwright';
let browser,page;
let stage="configuration",status=null;const blocked=[];
function target(raw){const u=new URL(raw);return {origin:u.origin,path:u.pathname}}
async function stop(){if(browser)await browser.close();process.exit(1)}
process.once('SIGTERM',stop);process.once('SIGINT',stop);
try {
 let raw='';for await(const chunk of process.stdin){raw+=chunk;if(Buffer.byteLength(raw)>65536)throw new Error('input_limit')}
 const input=JSON.parse(raw),origin=new URL(input.url).origin,config=input.browser,ignored=new Set(config.ignored_requests);
 const queries=config.json_requests===undefined?[]:config.json_requests;
 if(!process.env.PLATFORM_DRAW_BROWSER_EXECUTABLE||!input.allowed_origins.includes(origin)||!input.wait_selector||!Number.isInteger(input.timeout_ms)||input.timeout_ms<1000||input.timeout_ms>10000)throw new Error('invalid_configuration');
 const deadline=Date.now()+input.timeout_ms;
 const remaining=()=>{const ms=deadline-Date.now();if(ms<=0)throw new Error('render_timeout');return ms};
 stage="launch";browser=await chromium.launch({headless:true,chromiumSandbox:true,timeout:remaining(),executablePath:process.env.PLATFORM_DRAW_BROWSER_EXECUTABLE});
 const context=await browser.newContext({serviceWorkers:'block',acceptDownloads:false});

 await context.route('**/*',async route=>{
  const request=route.request(),url=new URL(request.url());
  if(ignored.has(url.origin+url.pathname)){await route.abort('blockedbyclient');return}
  const readPost=request.method()==='POST'&&url.origin===origin&&queries.some(q=>q.method==='POST'&&new URL(q.url).pathname===url.pathname);
  let redirected=0;for(let previous=request.redirectedFrom();previous;previous=previous.redirectedFrom())redirected++;
  const allowedRedirect=request.isNavigationRequest()&&url.origin===origin&&redirected<=(config.max_redirects===undefined?0:config.max_redirects);
  const reason=!['GET','HEAD'].includes(request.method())&&!readPost?'method':!['http:','https:'].includes(url.protocol)||!input.allowed_origins.includes(url.origin)?'origin':request.redirectedFrom()&&!allowedRedirect?'redirect':'';
  if(reason){if(blocked.length<12)blocked.push({reason,method:request.method(),type:request.resourceType(),...target(request.url())});await route.abort('blockedbyclient');return}
  // Credentials never follow a document to another origin.
  const headers={...request.headers()};delete headers.authorization;
  if(input.bearer&&url.origin===origin)headers.authorization='Bearer '+input.bearer;
  await route.continue({headers});
 });
 await context.routeWebSocket('**/*',socket=>{blocked.push({reason:'websocket'});socket.close()});
 page=await context.newPage();context.on('page',p=>{if(p!==page){blocked.push({reason:'popup'});void p.close()}});
 if(config.entry_url){const entry=new URL(config.entry_url);if(entry.origin!==origin)throw new Error('entry_origin');stage="entry";const entered=await page.goto(config.entry_url,{waitUntil:'domcontentloaded',timeout:remaining()});if(!entered||entered.status()!==200)throw new Error('entry_status')}
 stage="navigation";const response=await page.goto(input.url,{waitUntil:'domcontentloaded',timeout:remaining()});
 status=response?.status()??null;
 if(!response||response.status()!==200)throw new Error('document_status');
 stage="selector";await page.locator(input.wait_selector).first().waitFor({state:'attached',timeout:remaining()});
 if(blocked.length)throw new Error('blocked_request');
 stage="queries";const responses={};let size=0;
 for(const q of queries){if(new URL(q.url).origin!==origin)throw new Error('query_origin');const headers={};if(q.method==='POST')headers['Content-Type']='application/x-www-form-urlencoded';if(input.bearer)headers.Authorization='Bearer '+input.bearer;const response=await context.request.fetch(q.url,{method:q.method,headers,data:q.body,timeout:remaining(),maxRedirects:0});if(response.status()!==200)throw new Error('query_status');const body=await response.body();size+=body.length;if(size>1048576)throw new Error('response_limit');responses[q.id]=JSON.parse(body.toString('utf8'))}
 const html=await page.content();if(Buffer.byteLength(html)>1048576)throw new Error('response_limit');
 process.stdout.write(JSON.stringify({html,responses}));
} catch(error) {
 const net=String(error?.message??'').match(/net::[A-Z_]+/)?.[0]??null;
 const code=blocked.length?'blocked_request':error?.name==='TimeoutError'?'timeout':net?'network':status!==null&&status!==200?'http_status':'render_failed';
 let document=null;if(page){try{document={...target(page.url()),title:await page.title(),password_fields:await page.locator('input[type=password]').count()}}catch{}}
 process.stderr.write(JSON.stringify({code,stage,status,network:net,blocked,document})+'\n');process.exitCode=1;
} finally {if(browser)await browser.close()}
