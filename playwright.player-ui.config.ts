import {defineConfig} from '@playwright/test'
import layout from './playwright.layout.config'
const flows=['user-list-review.spec.ts','user-lists.spec.ts','results-fixed.spec.ts','player-pages.spec.ts','referral-tabs.spec.ts','user-withdrawal.spec.ts','user-page-titles.spec.ts','unified-feedback.spec.ts','dropdown-interactions.spec.ts','notification-bet.spec.ts']
export default defineConfig({...layout,outputDir:process.env.LOTTERY_LAYOUT_OUTPUT_DIR??'test-results/player-ui',reporter:[['list'],['json',{outputFile:`${process.env.LOTTERY_LAYOUT_OUTPUT_DIR??'test-results/player-ui'}/results.json`}]],projects:layout.projects!.map(project=>({...project,testMatch:flows.map(file=>file==='player-pages.spec.ts'&&project.name!.endsWith('reflow')?'player-pages-reflow.spec.ts':file)}))})
