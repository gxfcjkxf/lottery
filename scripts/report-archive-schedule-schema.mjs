export const archiveScheduleProperties = {
 daily_execution_time: {type:'string',pattern:'^([01][0-9]|2[0-3]):[0-5][0-9]$'},
 daily_period_offset: {type:'integer',minimum:1,maximum:365},
 monthly_execution_day: {type:'integer',minimum:1,maximum:31,description:'If this day does not exist in a month, execute on the last day.'},
 monthly_execution_time: {type:'string',pattern:'^([01][0-9]|2[0-3]):[0-5][0-9]$'},
 monthly_period_offset: {type:'integer',minimum:1,maximum:120},
};
