import { ApiError } from './api'
import { readApiEnvelope } from './api-response'
export interface AnnouncementCopy {title:string; html:string; button:string; url:string}
export interface AnnouncementConfig {
 content:Record<'en'|'zh-CN',AnnouncementCopy>; targets:string[]; audience:'all'|'guest'|'member'; popup:boolean; frequency:'session'|'daily'|'interval'|'once'; interval_days:number; allow_mute:boolean; pinned:boolean; sort:number; starts_at:string|null; ends_at:string|null
}
export interface Announcement {id:string;brand_id:string|null;version:number;generation:number;status:'draft'|'published'|'withdrawn';state:'draft'|'scheduled'|'active'|'ended'|'withdrawn';config:AnnouncementConfig;published_at:string|null;updated_at:string;eligible:boolean}
export interface AnnouncementInput {version:number;status:Announcement['status'];remind:boolean;config:AnnouncementConfig}
export const announcementAsset = /^\/api\/v1\/announcement-assets\/[a-f0-9]{64}\.(png|jpg|webp)$/
export function newAnnouncement():AnnouncementInput {return {version:0,status:'draft',remind:false,config:{content:{en:{title:'',html:'',button:'',url:''},'zh-CN':{title:'',html:'',button:'',url:''}},targets:[],audience:'all',popup:true,frequency:'daily',interval_days:1,allow_mute:true,pinned:false,sort:0,starts_at:null,ends_at:null}}}
export function safeAnnouncementLink(s:string):boolean {if(s.trim()!==s||/[\\\u0000-\u0020\u007f]/.test(s))return false;if(s.startsWith('/')&&!s.startsWith('//'))return true;try{const u=new URL(s);return u.protocol==='https:'&&!!u.hostname&&!u.username&&!u.password}catch{return false}}
// Client validation is also used for unsaved previews; published HTML is validated on the server.
export function safeAnnouncementHTML(source:string):string {
 const doc=new DOMParser().parseFromString(source,'text/html')
 if(doc.head.children.length)throw new ApiError('Unsupported announcement markup',400,'ANNOUNCEMENT_INVALID')
 const tags=new Set(['P','BR','H2','H3','STRONG','EM','U','S','UL','OL','LI','BLOCKQUOTE','A','IMG','MARK'])
 const walk=(node:Node)=>{if(node.nodeType===Node.TEXT_NODE)return;if(!(node instanceof HTMLElement)||!tags.has(node.tagName))throw new ApiError('Unsupported announcement markup',400,'ANNOUNCEMENT_INVALID')
  for(const a of [...node.attributes]){if(node.tagName==='A'&&a.name==='href'&&safeAnnouncementLink(a.value))continue;if(node.tagName==='A'&&a.name==='rel'&&a.value==='noopener noreferrer')continue;if(node.tagName==='IMG'&&a.name==='src'&&announcementAsset.test(a.value))continue;if(node.tagName==='IMG'&&a.name==='alt')continue;throw new ApiError('Unsupported announcement attribute',400,'ANNOUNCEMENT_INVALID')}
  if(node.tagName==='IMG'&&!announcementAsset.test(node.getAttribute('src')??''))throw new ApiError('Invalid announcement image',400,'ANNOUNCEMENT_INVALID')
  if(node.tagName==='A')node.setAttribute('rel','noopener noreferrer')
  for(const c of [...node.childNodes])walk(c)
 }
 for(const node of [...doc.body.childNodes])walk(node)
 return doc.body.innerHTML
}
function isItem(v:unknown):v is Announcement {if(!v||typeof v!=='object')return false;const x=v as Announcement;return typeof x.id==='string'&&Number.isSafeInteger(x.version)&&x.version>0&&Number.isSafeInteger(x.generation)&&!!x.config&&['draft','published','withdrawn'].includes(x.status)&&['draft','scheduled','active','ended','withdrawn'].includes(x.state)&&['en','zh-CN'].every(l=>{const c=x.config.content?.[l as 'en'];return c&&typeof c.title==='string'&&typeof c.html==='string'&&typeof c.button==='string'&&typeof c.url==='string'})}
export function createAnnouncementsApi(base:string,brand?:string,session?:string,fetcher:typeof fetch=fetch){
 async function request(path:string,method='GET',body?:unknown,key?:string){const headers=new Headers();if(brand)headers.set('X-Brand-ID',brand);if(session)headers.set('X-Announcement-Session',session);if(body!==undefined)headers.set('Content-Type','application/json');if(key)headers.set('Idempotency-Key',key)
  const response=await fetcher(base+path,{method,headers,credentials:'same-origin',...(body!==undefined?{body:JSON.stringify(body)}:{})});const envelope=await readApiEnvelope(response,(message,status,code)=>new ApiError(message,status,code))
  return envelope.data
 }
 return {
  async list(){const data=await request('/announcements') as {items:unknown[]};if(!data||!Array.isArray(data.items)||!data.items.every(isItem))throw new ApiError('Invalid announcements',502,'INVALID_RESPONSE');return data.items as Announcement[]},
  async brands(){const data=await request('/announcements/brands') as {items:{id:string;name:string}[]};if(!Array.isArray(data?.items)||!data.items.every(v=>typeof v.id==='string'&&typeof v.name==='string'))throw new ApiError('Invalid brands',502,'INVALID_RESPONSE');return data.items},
  async save(id:string,body:AnnouncementInput,key:string){const data=await request('/announcements'+(id?'/'+id:''),id?'PUT':'POST',body,key);if(!isItem(data))throw new ApiError('Invalid announcement receipt',502,'INVALID_RESPONSE');return data},
  async upload(file:File){const headers=new Headers({'Content-Type':file.type});if(brand)headers.set('X-Brand-ID',brand);const response=await fetcher(base+'/announcements/image',{method:'POST',credentials:'same-origin',headers,body:file});const envelope=await readApiEnvelope(response,(message,status,code)=>new ApiError(message,status,code));const url=(envelope.data as {url?:unknown})?.url;if(typeof url!=='string'||!announcementAsset.test(url))throw new ApiError('Invalid image receipt',502,'INVALID_RESPONSE');return url},
  receipt:(action:'view'|'mute'|'sync',items:{id:string;generation:number}[]=[])=>request('/announcements/receipts','POST',{action,items}),
 }
}
