import {describe,it,expect} from 'vitest'
import {announcementsCatalog} from './announcements-catalog'
import {createCatalogI18n} from './localization'
import {safeAnnouncementLink,createAnnouncementsApi,newAnnouncement} from './announcements'
describe('announcements',()=>{
 it('has no independent timezone in new announcement inputs',()=>{expect(newAnnouncement().config).not.toHaveProperty('timezone')})
 it('uses complete bilingual copy and rejects unsafe links',()=>{createCatalogI18n(announcementsCatalog);for(const url of ['javascript:alert(1)','//evil.example','/\\evil.example','https://user:password@example.com'])expect(safeAnnouncementLink(url)).toBe(false);for(const url of ['/announcements','https://example.com/news'])expect(safeAnnouncementLink(url)).toBe(true)})
 it('preserves idempotency and brand boundaries in save requests',async()=>{const calls:{url:string;headers:Headers;body:string}[]=[];const api=createAnnouncementsApi('/api/v1/admin','brand',undefined,async(url,init)=>{calls.push({url:String(url),headers:new Headers(init?.headers),body:String(init?.body)});return new Response(JSON.stringify({success:false,error:{code:'ANNOUNCEMENT_CONFLICT',message:'Changed'}}),{status:409})});await expect(api.save('id',newAnnouncement(),'original-key')).rejects.toMatchObject({code:'ANNOUNCEMENT_CONFLICT',status:409});expect(calls[0].url).toBe('/api/v1/admin/announcements/id');expect(calls[0].headers.get('X-Brand-ID')).toBe('brand');expect(calls[0].headers.get('Idempotency-Key')).toBe('original-key')})
})
