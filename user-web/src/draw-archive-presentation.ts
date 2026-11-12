export interface ArchiveResult { regular: number[]; special: number[]; digits: number[] }
export interface ArchiveAttribute { wave: string; zodiac: string; element: string }
export function archiveNumbers(result: ArchiveResult) {
  return [...result.regular.map(value=>({value,special:false})),...result.special.map((value,index)=>({value,special:index===0})),...result.digits.map(value=>({value,special:false}))]
}
// These labels use the existing Mark Six number attributes, including 49 as a push.
export function specialFacts(result: ArchiveResult, mode: 'special' | 'tail'): string[] {
  const n=result.special[0]!
  if(n===49) return mode==='special'?['push','push']:['push','push','push','push']
  const sum=Math.floor(n/10)+n%10,tail=n%10
  return mode==='special'?[n%2?'odd':'even',n>=25?'big':'small']:[sum>=7?'sum_big':'sum_small',sum%2?'sum_odd':'sum_even',tail>=5?'tail_big':'tail_small',tail%2?'odd':'even']
}
export function totalFacts(result: ArchiveResult, attributes: Record<string,ArchiveAttribute>) {
  const numbers=[...result.regular,...result.special]
  const total=numbers.reduce((sum,value)=>sum+value,0)
  const scores:Record<string,number>={red:0,blue:0,green:0}
  let complete=true
  result.regular.forEach(value=>{const wave=attributes[String(value).padStart(2,'0')]?.wave;if(wave in scores)scores[wave]!+=2;else complete=false})
  result.special.forEach(value=>{const wave=attributes[String(value).padStart(2,'0')]?.wave;if(wave in scores)scores[wave]!+=3;else complete=false})
  const maximum=Math.max(...Object.values(scores)),winners=Object.keys(scores).filter(key=>scores[key]===maximum)
  return {total,parity:total%2?'odd':'even',size:total>=175?'big':'small',wave:complete?(winners.length===1?winners[0]!:'push'):null}
}

export type ArchiveSort = 'drawn' | 'size'
export function displayedArchiveNumbers(result: ArchiveResult, regularOrder: number[] | null, sort: ArchiveSort) {
  if (sort === 'drawn' && regularOrder === null) return null
  const regular = sort === 'drawn' ? regularOrder! : [...result.regular].sort((a,b)=>a-b)
  return archiveNumbers({...result,regular})
}
export function numberFacts(number: number) {
  const sum=Math.floor(number/10)+number%10
  return {size:number>=25?'big':'small',parity:number%2?'odd':'even',sumParity:sum%2?'odd':'even'}
}
export function zodiacFamily(zodiac: string): 'domestic' | 'wild' | null {
  if (!zodiac) return null
  return ['goat','horse','ox','pig','dog','rooster'].includes(zodiac)?'domestic':'wild'
}

export function shortArchivePeriod(periodNo: string, date: string, daily: boolean) {
  return !daily && /^\d{4}-\d{2}-\d{2}$/.test(date) && new RegExp(`^${date.slice(0,4)}\\d+$`).test(periodNo) ? periodNo.slice(4) : periodNo
}
