import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { dirname, isAbsolute, join, resolve } from 'node:path'

// Walk past package.json files used only to set the module type in dist/.
export function packageRoot(moduleId) {
  const filename = moduleId.replace(/^\0/, '').split('?')[0]
  if (!isAbsolute(filename) || !filename.includes('/node_modules/')) return null
  let directory = dirname(filename)
  while (dirname(directory) !== directory) {
    const manifest = join(directory, 'package.json')
    if (existsSync(manifest)) {
      const pkg = JSON.parse(readFileSync(manifest, 'utf8'))
      if (pkg.name && pkg.version) return directory
    }
    directory = dirname(directory)
  }
  throw new Error(`Cannot identify bundled dependency: ${moduleId}`)
}

export function packageNotice(directory) {
  const pkg = JSON.parse(readFileSync(join(directory, 'package.json'), 'utf8'))
  const files = readdirSync(directory, { withFileTypes: true })
    .filter(file => file.isFile() && /^(licen[sc]e|copying|notice)(?:[.-]|$)/i.test(file.name))
    .map(file => file.name).sort()
  // A NOTICE or a package.json SPDX identifier does not replace a license.
  if (!files.some(file => /^(licen[sc]e|copying)(?:[.-]|$)/i.test(file))) {
    throw new Error(`Missing license text for bundled dependency ${pkg.name}@${pkg.version}`)
  }
  const sections = files.map(file => {
    const text = readFileSync(join(directory, file), 'utf8').trim()
    if (!text) throw new Error(`Empty license/notice: ${pkg.name}/${file}`)
    return `${file}\n\n${text}`
  })
  return `${pkg.name}@${pkg.version}\n${'='.repeat(72)}\n\n${sections.join('\n\n')}`
}

export default function thirdPartyNotices() {
  let root
  return {
    name: 'third-party-notices',
    apply: 'build',
    configResolved(config) { root = config.root },
    generateBundle(_options, bundle) {
      const packages = new Set()
      for (const output of Object.values(bundle)) {
        if (output.type !== 'chunk') continue
        for (const moduleId of Object.keys(output.modules)) {
          const directory = packageRoot(moduleId)
          if (directory) packages.add(directory)
        }
      }
      // Generated CSS and Vite's inline runtime helpers are not ordinary
      // node_modules entries in Rollup's emitted JavaScript module list.
      for (const name of ['tailwindcss', 'vite']) {
        packages.add(resolve(root, 'node_modules', name))
      }
      const notices = [...packages].map(packageNotice).sort()
      if ([...packages].some(directory =>
        JSON.parse(readFileSync(join(directory, 'package.json'), 'utf8')).name === 'lucide-react')) {
        const feather = readFileSync(resolve(root, 'licenses/LICENSE.feather'), 'utf8').trim()
        if (!feather) throw new Error('Missing Feather license for Lucide-derived icons')
        notices.push(`Feather icons incorporated into Lucide\n${'='.repeat(72)}\n\n${feather}`)
      }
      this.emitFile({
        type: 'asset',
        fileName: 'THIRD_PARTY_NOTICES.txt',
        source: 'SNI Gateway frontend — third-party licenses and notices\n\n' +
          'Generated from dependencies used by this build. Original license texts\n' +
          'are preserved below. Upstream license files may also cover build-time\n' +
          'components that are not included in the browser bundle. These terms\n' +
          'apply to their respective components, independently of the project MIT license.\n\n' +
          notices.join('\n\n\n') + '\n',
      })
    },
  }
}
