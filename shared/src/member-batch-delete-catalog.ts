import type { TranslationCatalog } from './localization'

export const memberBatchDeleteCatalog = {
  en: {
    selectPage: 'Select all', selectItem: 'Select {account}', selectAccount: 'Select {account}', selected: '{count} selected',
    titleRoles: 'Delete roles', confirmRoles: 'Delete {count} roles',
    titleBrands: 'Delete brands', confirmBrands: 'Delete {count} brands',
    delete: 'Delete selected', title: 'Delete accounts', cancel: 'Cancel', close: 'Close',
    confirm: 'Delete {count} accounts', retry: 'Retry unfinished', busy: 'Deleting…',
    pending: 'Pending', deleted: 'Deleted', failed: 'Failed', unknown: 'Unconfirmed',
    summary: '{success} deleted · {remaining} unfinished',
    uncertain: 'A deletion result is unconfirmed. Retry to confirm it before closing. The same request will be reused.',
  },
  'zh-CN': {
    selectPage: '全选', selectItem: '选择 {account}', selectAccount: '选择账号 {account}', selected: '已选 {count} 个',
    titleRoles: '批量删除角色', confirmRoles: '删除 {count} 个角色',
    titleBrands: '批量删除品牌', confirmBrands: '删除 {count} 个品牌',
    delete: '批量删除', title: '批量删除账号', cancel: '取消', close: '关闭',
    confirm: '删除 {count} 个账号', retry: '重试未完成项', busy: '删除中…',
    pending: '待删除', deleted: '已删除', failed: '删除失败', unknown: '结果待确认',
    summary: '已删除 {success} 个 · 未完成 {remaining} 个',
    uncertain: '部分删除结果尚未确认，请重试确认后再关闭。重试会沿用原请求。',
  },
} satisfies TranslationCatalog
