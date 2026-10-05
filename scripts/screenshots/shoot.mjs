// Takes the screenshots used in docs/USER_GUIDE.md.
//
// Needs: the mock servers + `wails dev` running (see docs/USER_GUIDE.md → "Regenerating
// the screenshots"), puppeteer-core installed, and a Chromium-based browser.
//
//   CHROME=/Applications/Brave\ Browser.app/Contents/MacOS/Brave\ Browser node scripts/screenshots/shoot.mjs
import puppeteer from 'puppeteer-core'
import { mkdirSync } from 'node:fs'

const OUT = process.env.OUT ?? 'docs/images'
const URL = process.env.URL ?? 'http://localhost:34115'
mkdirSync(OUT, { recursive: true })

const browser = await puppeteer.launch({
  executablePath: process.env.CHROME,
  headless: true,
  args: ['--no-first-run', '--no-default-browser-check', '--hide-scrollbars'],
  defaultViewport: { width: 1440, height: 880, deviceScaleFactor: 2 },
})
const page = await browser.newPage()
await page.evaluateOnNewDocument((theme) => {
  localStorage.setItem('syncscope.theme', JSON.stringify(theme))
  localStorage.setItem('syncscope.view', JSON.stringify('list'))
  localStorage.setItem('syncscope.product', JSON.stringify('cd'))
  localStorage.setItem('syncscope.sidebarCollapsed', 'false')
  localStorage.setItem('syncscope.sidebarWidth', '250')
}, process.env.THEME ?? 'light')

const wait = (ms) => new Promise((r) => setTimeout(r, ms))
const shot = async (name) => {
  await wait(400)
  await page.screenshot({ path: `${OUT}/${name}.png` })
  console.log('📸', name)
}
const click = (sel, text) =>
  page.evaluate((sel, text) => {
    const el = [...document.querySelectorAll(sel)].find((e) => !text || e.textContent.includes(text))
    if (!el) throw new Error(`not found: ${sel} ${text ?? ''}`)
    el.click()
  }, sel, text)
const dblclick = (sel, text) =>
  page.evaluate((sel, text) => {
    const el = [...document.querySelectorAll(sel)].find((e) => !text || e.textContent.includes(text))
    el.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
  }, sel, text)
const type = (sel, value) =>
  page.evaluate((sel, value) => {
    const el = document.querySelector(sel)
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  }, sel, value)
const back = () => click('.drawer .back')
const product = (name) => click('.product', name)
const tab = (name) => click('.tabs .tab', name)
const search = async (q) => { await type('.search input', q); await wait(500) }

await page.goto(URL, { waitUntil: 'networkidle0' })
await page.evaluate(async () => {
  await window.go.main.App.LoginPassword('stg01', 'admin', 'admin', true)
  await window.go.main.App.LoginPassword('prod01', 'admin', 'admin', true)
})
await wait(5000)

// --- Argo CD ---
await shot('01-applications')
await search('health:degraded -cluster:legacy')
await shot('02-search')
await search('')
await click('.seg.small button', 'Tiles')
await shot('03-tiles')
await click('.seg.small button', 'List')
await tab('Problems')
await wait(600)
await shot('04-problems')
await tab('ApplicationSets')
await wait(500)
await shot('05-applicationsets')
await click('table.grid tr.click', 'checkout')
await wait(1500)
await shot('06-applicationset-page')
await back()
await tab('Applications')
await search('health:degraded -cluster:legacy')
await click('.trow .cell.name')
await wait(2000)
await click('.rtree-toolbar .btn', 'Fit')
await shot('07-app-tree')
await dblclick('.rnode.degraded', 'pod')
await wait(2500)
await shot('08-resource-logs')
await click('.rnode', 'ab10c')
await wait(500)
await click('.rtree-side .seg button', 'terminal')
await wait(2500)
for (const cmd of ['whoami', 'env']) {
  await page.keyboard.type(cmd, { delay: 60 })
  await wait(300)
  await page.keyboard.press('Enter')
  await wait(600)
}
await shot('09-terminal')
await back()
await search('sync:outofsync is:ok')
await click('.trow .cell.name')
await wait(1200)
await click('.detail-tabs .tab', 'Diff')
await wait(1500)
await shot('10-diff')
await click('.detail-tabs .tab', 'History')
await wait(1500)
await shot('11-history')
await back()
await search('name:platform-addons -legacy')
await click('.trow .cell.name')
await wait(1200)
await click('.detail-tabs .tab', 'Parameters')
await wait(1500)
await shot('12-parameters')
await back()
await search('appset:"checkout" cluster:dev')
await click('.thead input[type=checkbox]')
await wait(300)
await click('.actionbar .btn', 'Sync')
await wait(500)
await shot('13-bulk-sync')
await page.keyboard.press('Escape')
await wait(300)
await click('.actionbar a', 'clear')
await search('')

// --- Workflows / Rollouts / Events ---
await product('Workflows')
await wait(800)
await shot('14-workflows')
await click('.trow', 'Failed') .catch(() => {})
await page.evaluate(() => [...document.querySelectorAll('.trow')].find((r) => r.className.includes('sev-2'))?.click())
await wait(1500)
await shot('15-workflow-graph')
await page.evaluate(() => [...document.querySelectorAll('.wf-node')].find((n) => n.className.includes('degraded') && !n.textContent.includes('DAG'))?.click())
await wait(2500)
await shot('16-workflow-step-logs')
await back()
await product('Rollouts')
await wait(800)
await shot('17-rollouts')
await page.evaluate(() => [...document.querySelectorAll('.trow')].find((r) => r.textContent.includes('step 1/5'))?.click())
await wait(1500)
await shot('18-rollout')
await back()
await product('Events')
await wait(1000)
await shot('19-events-flow')

// --- palette & settings ---
await product('Argo CD')
await page.evaluate(() => document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', metaKey: true, bubbles: true })))
await wait(400)
await type('.palette input', 'checkout')
await wait(400)
await shot('20-palette')
await page.keyboard.press('Escape')
await click('.footer-icons .rail-btn')
await wait(500)
await shot('21-settings-instances')
await click('.settings-nav a', 'Kubernetes')
await wait(800)
// show the default kubeconfig location instead of the demo's temp file
await page.evaluate(() => document.querySelectorAll('.settings-content .muted-sm .mono').forEach((e) => { e.textContent = '~/.kube/config' }))
await shot('22-settings-kubernetes')
await click('.settings-nav a', 'Argo CD configuration')
await wait(1500)
await shot('23-settings-argocd')
await click('.settings-content .btn', 'Add instance').catch(async () => { await click('.settings-nav a', 'Argo CD instances'); await wait(300); await click('.settings-content .btn', 'Add instance') })
await wait(500)
await shot('24-add-instance')
await page.keyboard.press('Escape')
await page.keyboard.press('Escape')

await browser.close()
