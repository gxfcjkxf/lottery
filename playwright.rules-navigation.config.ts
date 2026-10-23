import { defineConfig } from '@playwright/test'
import base from './playwright.configured-rules.config'

export default defineConfig({
  ...base,
  testMatch: ['rules-navigation.spec.ts', 'platform-game-readiness.spec.ts', 'draw-config.spec.ts'],
  outputDir: 'test-results/rules-navigation',
  projects: base.projects!.map(project => ({ ...project, use: { ...project.use, browserName: project.name!.startsWith('ios') ? 'webkit' as const : 'chromium' as const } })),
})
