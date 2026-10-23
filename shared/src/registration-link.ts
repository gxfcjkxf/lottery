import { normalizeJoinCodeFields } from './auth'
export function registrationShareLink(origin:string,kind:'agent'|'referral',code:string):string {
  const base=new URL(origin)
  if(!['http:','https:'].includes(base.protocol)||base.username||base.password||base.pathname!=='/'||base.search||base.hash)throw new TypeError('Invalid user application origin')
  const fields=normalizeJoinCodeFields(kind==='agent'?{agent_code:code}:{referral_code:code})
  if(!Object.keys(fields).length)throw new TypeError('Join code is required')
  const url=new URL('/register',base)
  for(const [key,value] of Object.entries(fields))url.searchParams.set(key,value)
  return url.toString()
}
