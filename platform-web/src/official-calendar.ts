export function calendarWeeks(month:string):Array<Array<{date:string;day:number;inMonth:boolean}>>{
 const [year,m]=month.split('-').map(Number)
 const first=new Date(Date.UTC(year!,m!-1,1)),days=new Date(Date.UTC(year!,m!,0)).getUTCDate()
 const count=Math.ceil((first.getUTCDay()+days)/7)*7
 return Array.from({length:count/7},(_,week)=>Array.from({length:7},(_,weekday)=>{
  const date=new Date(Date.UTC(year!,m!-1,1-first.getUTCDay()+week*7+weekday))
  return {date:date.toISOString().slice(0,10),day:date.getUTCDate(),inMonth:date.getUTCMonth()===m!-1}
 }))
}
export function shiftCalendarMonth(month:string,offset:number):string{
 const [year,m]=month.split('-').map(Number)
 return new Date(Date.UTC(year!,m!-1+offset,1)).toISOString().slice(0,7)
}
