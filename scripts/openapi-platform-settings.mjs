const ref = name => ({ $ref: `#/components/schemas/${name}` });
const fields = { version: ref('PositiveInteger'), display_name: { type: 'string' }, logo_text: { type: 'string' }, logo_url: { type: 'string' }, timezone: { type: 'string' }, date_format: { enum: ['YYYY-MM-DD', 'DD/MM/YYYY', 'MM/DD/YYYY'] }, time_format: { enum: ['HH:mm:ss', 'hh:mm:ss A'] } };
export const schemas = {
  PlatformPresentation: {type:'object',properties:{display_name:{type:'string'},logo_text:{type:'string'},logo_url:{type:'string'}},required:['display_name','logo_text','logo_url'],additionalProperties:false},
  PlatformSettingsInput: { type: 'object', properties: fields, required: Object.keys(fields), additionalProperties: false },
  PlatformSettings: { type: 'object', properties: { ...fields, updated_at: ref('DateTime'), audit_log_id: ref('UUID') }, required: [...Object.keys(fields), 'updated_at'], additionalProperties: false },
};
export const operations = [
  {method:'GET',path:'/api/v1/platform/presentation',operationId:'platformPresentation',summary:'Read public platform branding',tag:'platform-settings',auth:'public',data:ref('PlatformPresentation'),description:'Public branding only, with Cache-Control: no-store. Administrative settings and audit metadata are excluded.'},
  { method: 'GET', path: '/api/v1/platform/platform-settings', operationId: 'platformSettingsRead', summary: 'Read platform display settings', tag: 'platform-settings', auth: 'admin', permissions: ['platform_settings.view.platform'], data: ref('PlatformSettings') },
  { method: 'PATCH', path: '/api/v1/platform/platform-settings', operationId: 'platformSettingsUpdate', summary: 'Update platform display settings with a version check', tag: 'platform-settings', auth: 'admin', permissions: ['platform_settings.write.platform'], data: ref('PlatformSettings'), requestBody: ref('PlatformSettingsInput'), idempotency: true },
];
