import base from './playwright.layout.config'
import { defineConfig } from '@playwright/test'
const port = Number(process.env.LOTTERY_LAYOUT_PORT_BASE ?? 15273) + 1
export default defineConfig({...base,outputDir:'test-results/commission-candidates',reporter:'list',projects:base.projects!.map(project=>({...project,testMatch:['commission-candidates.spec.ts']})),webServer:[{command:`pnpm --filter @lottery/admin-web exec vite --host 127.0.0.1 --port ${port} --strictPort`,url:`http://127.0.0.1:${port}`,reuseExistingServer:false}]})
