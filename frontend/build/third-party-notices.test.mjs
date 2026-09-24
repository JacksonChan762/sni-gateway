import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import thirdPartyNotices, { packageNotice, packageRoot } from './third-party-notices.mjs'

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'sni-notices-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const pkg = (name, license = 'License text', notice = '') => {
    const dir = join(root, 'node_modules', name)
    mkdirSync(dir, { recursive: true })
    writeFileSync(join(dir, 'package.json'), JSON.stringify({ name, version: '1.2.3' }))
    if (license !== null) writeFileSync(join(dir, 'LICENSE'), license)
    if (notice) writeFileSync(join(dir, 'NOTICE'), notice)
    return dir
  }
  return { root, pkg }
}

test('finds scoped packages past nested module-type manifests', t => {
  const { pkg } = fixture(t)
  const dir = pkg('@example/lib')
  mkdirSync(join(dir, 'dist'))
  writeFileSync(join(dir, 'dist/package.json'), '{"type":"module"}')
  assert.equal(packageRoot(`\0${dir}/dist/index.js?commonjs-proxy`), dir)
  assert.equal(packageRoot('/src/App.tsx'), null)
})

test('preserves license and NOTICE; missing or empty license blocks publication', t => {
  const { pkg } = fixture(t)
  const text = packageNotice(pkg('valid', 'Full permission and copyright', 'Extra attribution'))
  assert.match(text, /valid@1\.2\.3/)
  assert.match(text, /Full permission and copyright/)
  assert.match(text, /Extra attribution/)
  assert.throws(() => packageNotice(pkg('missing', null, 'NOTICE alone')), /Missing license/)
  assert.throws(() => packageNotice(pkg('empty', '')), /Empty license/)
})

test('emits notices for actual chunks, CSS, helpers and Feather; deduplicates packages', t => {
  const { root, pkg } = fixture(t)
  const lucide = pkg('lucide-react', 'Lucide ISC text')
  pkg('tailwindcss', 'Tailwind MIT text')
  pkg('vite', 'Vite MIT text')
  pkg('unused', 'Should not be included')
  mkdirSync(join(root, 'licenses'))
  writeFileSync(join(root, 'licenses/LICENSE.feather'), 'Feather MIT text')
  const plugin = thirdPartyNotices()
  plugin.configResolved({ root })
  let asset
  plugin.generateBundle.call({ emitFile(value) { asset = value } }, {}, {
    main: { type: 'chunk', modules: { [`${lucide}/a.js`]: {}, [`${lucide}/b.js`]: {} } },
    css: { type: 'asset', source: 'body {}' },
  })
  assert.equal(asset.fileName, 'THIRD_PARTY_NOTICES.txt')
  assert.equal(asset.source.match(/lucide-react@1\.2\.3/g).length, 1)
  for (const text of ['Lucide ISC text', 'Tailwind MIT text', 'Vite MIT text', 'Feather MIT text']) {
    assert.ok(asset.source.includes(text), text)
  }
  assert.ok(!asset.source.includes('Should not be included'))
})
