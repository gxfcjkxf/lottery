import {defineConfig} from '@playwright/test'
import layout from './playwright.layout.config'
export default defineConfig({...layout,outputDir:'test-results/draw-result-entry',reporter:[['list']],projects:layout.projects!.map(project=>({...project,testMatch:'draw-result-entry.spec.ts'}))})
