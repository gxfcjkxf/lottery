import { readApiEnvelope } from '@lottery/shared'
export class AgentError extends Error {constructor(message:string,readonly status:number,readonly code:string){super(message)}}
export const brandCode = new URLSearchParams(location.search).get('brand')
export const base = brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}/agent` : '/api/v1/agent'
export async function request<T>(path:string,body?:unknown,key?:string):Promise<T>{
 const response=await fetch(base+path,{method:body===undefined?'GET':'POST',credentials:'same-origin',headers:{Accept:'application/json',...(body===undefined?{}:{'Content-Type':'application/json'}),...(key?{'Idempotency-Key':key}:{})},...(body===undefined?{}:{body:JSON.stringify(body)})})
 return (await readApiEnvelope<T>(response,(m,s,c)=>new AgentError(m,s,c))).data
}
