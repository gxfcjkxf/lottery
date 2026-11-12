import {defineConfig} from '@playwright/test';
import base from './playwright.cash.config';
export default defineConfig({...base,testMatch:['cash-wallet.spec.ts','announcements.spec.ts','brand-contacts.spec.ts','account-search.spec.ts','bet-distribution.spec.ts','device.spec.ts','refresh-controls.spec.ts'],outputDir:'.local/static-fixes-20261111/browser-results',reporter:[['list'],['json',{outputFile:'.local/static-fixes-20261111/browser-results.json'}]]});
