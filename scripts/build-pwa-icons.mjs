import { mkdir, readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const executablePath = process.argv[2] ?? process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH

if (!executablePath || !path.isAbsolute(executablePath)) {
  throw new Error('Pass an absolute Chromium executable path as the first argument or set PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH')
}

const targets = [
  { app: 'user-web', background: '#20594c', size: 192, filename: 'icon-192.png' },
  { app: 'user-web', background: '#20594c', size: 512, filename: 'icon-512.png' },
  { app: 'user-web', background: '#20594c', size: 180, filename: 'apple-touch-icon.png' },
  { app: 'admin-web', background: '#263f67', size: 192, filename: 'icon-192.png' },
  { app: 'admin-web', background: '#263f67', size: 512, filename: 'icon-512.png' },
  { app: 'admin-web', background: '#263f67', size: 180, filename: 'apple-touch-icon.png' },
  { app: 'platform-web', background: '#17365c', size: 192, filename: 'icon-192.png' },
  { app: 'platform-web', background: '#17365c', size: 512, filename: 'icon-512.png' },
  { app: 'platform-web', background: '#17365c', size: 180, filename: 'apple-touch-icon.png' },
]

const browser = await chromium.launch({ executablePath })

try {
  for (const target of targets) {
    const svgPath = path.join(root, target.app, 'public', 'favicon.svg')
    const outputDirectory = path.join(root, target.app, 'public', 'icons')
    const svg = await readFile(svgPath, 'utf8')
    const page = await browser.newPage({
      viewport: { width: target.size, height: target.size },
      deviceScaleFactor: 1,
    })

    try {
      await page.setContent(`<!doctype html>
<html><head><meta name="viewport" content="width=device-width, initial-scale=1">
<style>html,body{margin:0;width:100%;height:100%;overflow:hidden;background:${target.background}}svg{display:block;width:100vw;height:100vh}svg>rect:first-of-type{fill:${target.background}!important}</style>
</head><body>${svg}</body></html>`, { waitUntil: 'load' })
      await mkdir(outputDirectory, { recursive: true })
      await page.screenshot({
        path: path.join(outputDirectory, target.filename),
        type: 'png',
        fullPage: true,
        omitBackground: false,
      })
    } finally {
      await page.close()
    }
  }
} finally {
  await browser.close()
}
