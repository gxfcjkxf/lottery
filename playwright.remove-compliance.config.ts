import { defineConfig } from '@playwright/test'
import base from './playwright.cash.config'
export default defineConfig({...base,testMatch:'security-inheritance.spec.ts',outputDir:'.local/remove-compliance/browser',reporter:[['list']],webServer:base.webServer})
