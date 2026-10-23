import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {resolve} from 'node:path';
import {chromium} from 'playwright';
async function server(handler){const s=createServer(handler);await new Promise(r=>s.listen(0,'127.0.0.1',r));return {url:`http://127.0.0.1:${s.address().port}`,close:()=>new Promise(r=>s.close(r))}}
function render(input,env={}){return new Promise((resolveResult,reject)=>{const p=spawn(process.execPath,[resolve('scripts/render-draw-feed.mjs')],{env:{...process.env,PLATFORM_DRAW_BROWSER_EXECUTABLE:chromium.executablePath(),...env},stdio:['pipe','pipe','pipe']});let stdout='',stderr='';p.stdout.on('data',c=>stdout+=c);p.stderr.on('data',c=>stderr+=c);p.on('error',reject);p.on('close',code=>resolveResult({code,stdout,stderr}));p.stdin.end(JSON.stringify({...input,browser:{wait_selector:input.wait_selector,timeout_ms:input.timeout_ms,...input.browser}}))})}
test('JavaScript fetch and delayed DOM creation render through configuration only',async()=>{
 let requests=0;
 const s=await server((req,res)=>{if(req.url==='/data'){requests++;assert.equal(req.headers.authorization,'Bearer local-test-token');res.setHeader('content-type','application/json');res.end(JSON.stringify({issue:'dynamic-7',numbers:[1,2,3,4,5,6,31]}));return}res.setHeader('content-type','text/html');res.end(`<main id="root"></main><script>fetch('/data').then(r=>r.json()).then(data=>{const root=document.querySelector('#root');root.innerHTML='<b id="issue">'+data.issue+'</b>'+data.numbers.map(n=>'<span class="ball">'+n+'</span>').join('');root.dataset.ready='yes'})</script>`)});
 try{const result=await render({url:s.url,wait_selector:'[data-ready="yes"]',timeout_ms:5000,allowed_origins:[s.url],bearer:'local-test-token'});assert.equal(result.code,0,result.stderr);const {html}=JSON.parse(result.stdout);assert.match(html,/<b id="issue">dynamic-7<\/b>/);assert.match(html,/<span class="ball">31<\/span>/);assert.equal(requests,1);assert.ok(!result.stdout.includes('local-test-token'))}finally{await s.close()}
});
test('unapproved subresources and redirects fail instead of yielding static HTML',async()=>{
 let leaked=0;const other=await server((_req,res)=>{leaked++;res.end('forbidden')});
 const s=await server((req,res)=>{if(req.url==='/redirect'){res.writeHead(302,{location:'/page'});res.end();return}res.setHeader('content-type','text/html');res.end(`<div id="ready"></div><script src="${other.url}/script"></script>`)});
 try{for(const path of ['/page','/redirect']){const result=await render({url:s.url+path,wait_selector:'#ready',timeout_ms:1500,allowed_origins:[s.url],bearer:''});assert.notEqual(result.code,0);assert.equal(result.stdout,'')}assert.equal(leaked,0)}finally{await s.close();await other.close()}
});
test('missing ready selectors time out and browser runtime absence is explicit',async()=>{
 const s=await server((_req,res)=>{res.setHeader('content-type','text/html');res.end('<div id="placeholder"></div>')});
 try{const input={url:s.url,wait_selector:'#ready',timeout_ms:1000,allowed_origins:[s.url],bearer:''};const result=await render(input);assert.notEqual(result.code,0);assert.equal(result.stdout,'');const unavailable=await render(input,{PLATFORM_DRAW_BROWSER_EXECUTABLE:''});assert.notEqual(unavailable.code,0);assert.equal(unavailable.stdout,'')}finally{await s.close()}
});
test('approved cross-origin scripts receive no source bearer credential',async()=>{
 let count=0;const other=await server((req,res)=>{assert.equal(req.headers.authorization,undefined);count++;res.setHeader('content-type','application/javascript');res.end('document.documentElement.dataset.ready="yes"')});
 const s=await server((req,res)=>{assert.equal(req.headers.authorization,'Bearer source-only');res.setHeader('content-type','text/html');res.end(`<script src="${other.url}/loader"></script>`)});
 try{const result=await render({url:s.url,wait_selector:'html[data-ready=yes]',timeout_ms:5000,allowed_origins:[s.url,other.url],bearer:'source-only'});assert.equal(result.code,0,result.stderr);assert.equal(count,1)}finally{await s.close();await other.close()}
});
test('configured entry, redirects, ignored telemetry and same-session JSON queries',async()=>{
 let ignored=0,queries=0;
 const s=await server((req,res)=>{
  if(req.url==='/entry'){res.writeHead(302,{'set-cookie':'source_session=ready; Path=/','location':'/welcome'});res.end();return}
  if(req.url==='/welcome'){res.end('ready');return}
  if(req.url==='/telemetry'){ignored++;res.end('{}');return}
  if(req.url==='/history'){assert.match(req.headers.cookie??'',/source_session=ready/);assert.equal(req.method,'POST');let body='';req.on('data',v=>body+=v);req.on('end',()=>{assert.equal(body,'top=1');queries++;res.setHeader('content-type','application/json');res.end('{"issue":"r7","at":"2026-10-19 21:30:00"}')});return}
  assert.match(req.headers.cookie??'',/source_session=ready/);res.setHeader('content-type','text/html');res.end('<div id="ready"><i class="ball_39 ">&nbsp;</i></div><script>fetch("/telemetry",{method:"POST"})</script>');
 });
 try{const r=await render({url:s.url+'/page',wait_selector:'#ready',timeout_ms:5000,allowed_origins:[s.url],bearer:'',browser:{entry_url:s.url+'/entry',max_redirects:1,ignored_requests:[s.url+'/telemetry'],json_requests:[{id:'history',url:s.url+'/history',method:'POST',body:'top=1'}]}});assert.equal(r.code,0,r.stderr);const out=JSON.parse(r.stdout);assert.equal(out.responses.history.at,'2026-10-19 21:30:00');assert.match(out.html,/ball_39/);assert.equal(ignored,0);assert.equal(queries,1)}finally{await s.close()}
});
