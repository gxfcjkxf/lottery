import { defineConfig } from '@playwright/test'
import base from './playwright.layout.config'
export default defineConfig({...base,workers:2,testMatch:'announcements.spec.ts',outputDir:'test-results/announcements',reporter:[['list'],['json',{outputFile:'test-results/announcements/results.json'}]],projects:base.projects!.map(project=>({...project,testMatch:'announcements.spec.ts'}))})
