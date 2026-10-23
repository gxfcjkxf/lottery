import {spawn} from 'node:child_process'
import {createWriteStream} from 'node:fs'
import {access, mkdir, rm, writeFile} from 'node:fs/promises'
import {setTimeout as delay} from 'node:timers/promises'
import {resolve} from 'node:path'

const databaseURL=process.env.TEST_DATABASE_URL
if(!databaseURL || !new URL(databaseURL).pathname.includes('_test_'))throw Error('TEST_DATABASE_URL must name an owned _test_ database')
const folder=resolve('.local/agent-browser')
await mkdir(folder,{recursive:true})
function start(command,args,options,log){
 const output=createWriteStream(log)
 const child=spawn(command,args,{...options,stdio:['ignore','pipe','pipe']})
 child.stdout.pipe(output,{end:false});child.stderr.pipe(output,{end:false})
 const done=new Promise((resolve,reject)=>{child.once('error',reject);child.once('close',code=>{output.end();resolve(code)})})
 return {child,done}
}
// Each device owns a fresh schema and API. Cumulative browser authentication
// must not consume another device's real IP/account limiter budget.
for(const project of ['android','android-landscape','ios','ios-landscape','ios-320','desktop']){
 const file=resolve(folder,project+'.json')
 await rm(file,{force:true});await rm(file+'.stop',{force:true})
 const env={...process.env,AGENT_BROWSER_FIXTURE_FILE:file}
 const fixture=start(process.env.LOTTERY_GO_BIN??'go',['test','-count=1','-timeout=5m','./internal/httpapi','-run','^TestIndependentAgentBrowserFixture$'],{cwd:resolve('backend'),env},resolve(folder,project+'-fixture.log'))
 let browserCode
 try{
  const deadline=Date.now()+60_000
  for(;;){
   try{await access(file);break}catch(error){if(error.code!=='ENOENT')throw error}
   if(fixture.child.exitCode!==null)throw Error('Agent fixture exited before becoming ready: '+project)
   if(Date.now()>deadline)throw Error('Agent fixture did not become ready: '+project)
   await delay(100)
  }
  const browser=start('pnpm',['exec','playwright','test','--config','playwright.agent.config.ts','--project='+project],{env},resolve(folder,project+'-browser.log'))
  browserCode=await browser.done
 }finally{
  await writeFile(file+'.stop','stop')
  const fixtureCode=await fixture.done
  await rm(file+'.stop',{force:true})
  if(fixtureCode!==0)throw Error('Agent fixture or schema cleanup failed: '+project)
 }
 if(browserCode!==0)throw Error('Agent browser regression failed: '+project)
 console.log(project+': passed; owned schema cleaned')
}
