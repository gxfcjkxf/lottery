import test from 'node:test'
import assert from 'node:assert/strict'
import {readFileSync,readdirSync,existsSync} from 'node:fs'
import {createRequire} from 'node:module'
import {resolve,dirname,extname,relative} from 'node:path'

const require=createRequire(resolve('user-web/package.json'))
const {parse}=require('vue/compiler-sfc')
const postcss=createRequire(require.resolve('vite'))('postcss')

test('player styles and imported components take colors from the theme palette',()=>{
 const seen=new Set(),errors=[]
 // Ball/chip highlights model their material; QR codes require a white backing.
 const materials=new Set(['shared/src/mark-six-balls.css','shared/src/quick-stake-chips.css'])
 function visit(file){
  file=resolve(file)
  if(seen.has(file)||!existsSync(file))return
  seen.add(file)
  const source=readFileSync(file,'utf8'),path=relative(process.cwd(),file)
  for(const match of source.matchAll(/(?:from\s*|import\s*|@import\s*|src=)["']([^"']+\.(?:vue|css))["']/g)){
   const id=match[1]
   if(id.startsWith('@lottery/shared/'))visit('shared/src/'+id.slice(16))
   else if(id.startsWith('.'))visit(resolve(dirname(file),id))
  }
  const styles=extname(file)==='.css'?[source]:extname(file)==='.vue'?parse(source).descriptor.styles.map(style=>style.content):[]
  for(const css of styles)postcss.parse(css).walkDecls(declaration=>{
   if(path==='shared/src/themes.css'||declaration.prop.startsWith('--')&&['shared/src/styles.css','shared/src/mark-six-balls.css'].includes(path))return
   const {prop,value}=declaration
   if(prop==='color'&&/^var\(--(?:surface|canvas)\)$/.test(value))errors.push(`${path}: use --on-primary for filled controls`)
   if(prop==='color-scheme'&&value!=='inherit')errors.push(`${path}: ${prop}: ${value}`)
   if(!/#[\da-f]{3,8}\b|\brgba?\(|(?:stroke|fill)=['"](?:white|black|%23)/i.test(value)&&! /^(white|black)$/.test(value))return
   if(materials.has(path)&&['background','box-shadow','text-shadow'].includes(prop))return
   if(path==='shared/src/CashAddress.vue'&&declaration.parent.selector==='.cash-qr'&&prop==='background'&&value==='white')return
   errors.push(`${path}: ${declaration.parent.selector} { ${prop}: ${value} }`)
  })
 }
 function directory(path){for(const entry of readdirSync(path,{withFileTypes:true})){const file=path+'/'+entry.name;if(entry.isDirectory())directory(file);else if(/\.(vue|css|ts)$/.test(file)&&!file.includes('.test.'))visit(file)}}
 directory('user-web/src')
 assert.deepEqual(errors,[])
})
