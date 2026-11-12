import {defineConfig} from '@playwright/test'
import base from './playwright.cash.config'
export default defineConfig({...base,testMatch:['announcements.spec.ts','modal-cancel.spec.ts'],outputDir:'.local/announcement-zone/browser-results',reporter:[['list'],['json',{outputFile:'.local/announcement-zone/browser-results.json'}]]})
