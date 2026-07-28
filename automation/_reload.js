const { chromium } = require('playwright')
const PORT = process.argv[2] || '9336'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
;(async () => {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${PORT}`)
  const ctx = browser.contexts()[0]
  const page = ctx.pages().find((p) => p.url().includes('checkout')) || ctx.pages()[ctx.pages().length - 1]
  console.log('reload:', page.url())
  await page.reload({ waitUntil: 'domcontentloaded' }).catch(() => {})
  await sleep(15000) // 给 Stripe Payment Element 充分渲染时间

  for (const f of page.frames()) {
    let items = []
    try {
      items = await f.$$eval('input,select', (els) => els.map((e) => ({
        name: e.name || '', type: e.type || '', id: e.id || '',
        ac: e.getAttribute('autocomplete') || '', ph: e.getAttribute('placeholder') || '', al: e.getAttribute('aria-label') || '',
      })))
    } catch (e) {}
    if (items.length) {
      console.log(`--- frame: ${(f.url() || '(blank)').slice(0, 72)}`)
      items.forEach((i) => console.log('   ', JSON.stringify(i)))
    }
  }
  await browser.close()
})().catch((e) => console.error('ERR', e.message))
