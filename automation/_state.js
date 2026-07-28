const { chromium } = require('playwright')
const PORT = process.argv[2] || '9336'
;(async () => {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${PORT}`)
  const ctx = browser.contexts()[0]
  const page = ctx.pages().find((p) => p.url().includes('checkout')) || ctx.pages()[ctx.pages().length - 1]
  console.log('URL:', page.url())
  const txt = await page.locator('body').innerText().catch(() => '')
  console.log('\n=== 可见文字(前 800) ===\n' + txt.slice(0, 800))
  const btns = await page.$$eval('button,[role=button],a', (els) => [...new Set(els.map((e) => (e.innerText || e.getAttribute('aria-label') || '').trim()).filter(Boolean))]).catch(() => [])
  console.log('\n=== 按钮/链接 ===', JSON.stringify(btns))
  console.log('\n=== iframes ===')
  page.frames().forEach((f) => { if (f !== page.mainFrame()) console.log('  ', (f.url() || '(blank)').slice(0, 80)) })
  await browser.close()
})().catch((e) => console.error('ERR', e.message))
