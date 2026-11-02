import { defineConfig } from '@playwright/test'
import layout from './playwright.layout.config'
export default defineConfig({ ...layout, outputDir: 'test-results/system-notes', projects: layout.projects!.map(project => ({ ...project, testMatch: 'system-notes.spec.ts' })) })
