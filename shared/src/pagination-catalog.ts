import type { TranslationCatalog } from './localization'

export const paginationCatalog = {
  en: {
    navigation: 'Pagination', previous: 'Previous page', next: 'Next page', first: 'First', last: 'Last', total: 'Total: {total}',
    page: 'Page {page}', jump: 'Go to', jumpPage: 'Go to page', pageUnit: 'page', go: 'GO', pageSize: '{count} / page', pageSizeLabel: 'Records per page',
    summary: 'Page {page} · {count} records on this page · Total: {total}',
    summarySingle: 'Page {page} · {count} record on this page · Total: {total}',
  },
  'zh-CN': {
    navigation: '分页', previous: '上一页', next: '下一页', first: '首页', last: '尾页', total: '共 {total}',
    page: '第 {page} 页', jump: '跳至', jumpPage: '跳转页码', pageUnit: '页', go: 'GO', pageSize: '{count} 条/页', pageSizeLabel: '每页条数',
    summary: '第 {page} 页 · 本页 {count} 条 · 共 {total} 条',
    summarySingle: '第 {page} 页 · 本页 {count} 条 · 共 {total} 条',
  },
} satisfies TranslationCatalog
