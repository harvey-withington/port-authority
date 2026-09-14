// Renders the app icon from its single SVG source into every raster the
// desktop build and the website need. Runs before `vite build` and
// `vite dev` (see package.json) and is a no-op when nothing is stale, so
// the SVG is the only file to edit.
import { copyFileSync, mkdirSync, statSync, writeFileSync } from 'fs'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'
import sharp from 'sharp'
import pngToIco from 'png-to-ico'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const projectRoot = resolve(root, '..')
const source = resolve(root, 'src/assets/images/pa-icon.svg')
const icoSizes = [16, 32, 48, 64, 128, 256]

const targets = [
  { path: resolve(root, 'public/pa-icon.svg'), type: 'copy' },
  { path: resolve(root, 'public/icon.ico'), type: 'ico' },
  { path: resolve(projectRoot, 'build/appicon.png'), type: 'png', size: 1024 },
  { path: resolve(projectRoot, 'build/windows/icon.ico'), type: 'ico' },
  { path: resolve(projectRoot, 'website/pa-icon.svg'), type: 'copy' },
  { path: resolve(projectRoot, 'website/pa-icon-512.png'), type: 'png', size: 512 },
]

function isStale() {
  let sourceTime
  try {
    sourceTime = statSync(source).mtimeMs
  } catch {
    console.error(`Icon source not found: ${source}`)
    process.exit(1)
  }
  return targets.some((t) => {
    try {
      return statSync(t.path).mtimeMs < sourceTime
    } catch {
      return true
    }
  })
}

function renderPng(size) {
  return sharp(source, { density: 300 }).resize(size, size).png().toBuffer()
}

async function main() {
  if (!isStale()) {
    console.log('Icons up to date, skipping.')
    return
  }
  console.log('Generating icons from', source)
  for (const target of targets) {
    mkdirSync(dirname(target.path), { recursive: true })
    switch (target.type) {
      case 'copy':
        copyFileSync(source, target.path)
        break
      case 'png':
        writeFileSync(target.path, await renderPng(target.size))
        break
      case 'ico':
        writeFileSync(target.path, await pngToIco(await Promise.all(icoSizes.map(renderPng))))
        break
    }
    console.log(`  ${target.type.padEnd(4)} -> ${target.path}`)
  }
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
