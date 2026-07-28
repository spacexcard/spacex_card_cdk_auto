// 连上正在运行的无痕 Chrome，逐个 frame dump 所有 input（找出 Stripe 卡号字段真实 name）
const { chromium } = require('playwright')
const PORT = process.argv[2] || '9336'
;(async () => {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${PORT}`)
  const ctx = browser.contexts()[0]
  const page = ctx.pages().find((p) => p.url().includes('checkout')) || ctx.pages()[ctx.pages().length - 1]
  console.log('URL:', page.url(), '\n')
  for (const f of page.frames()) {
    let inputs = []
    try {
      inputs = await f.$$eval('input,select,button', (els) => els.map((e) => ({
        tag: e.tagName.toLowerCase(), name: e.name || '', type: e.type || '', id: e.id || '',
        ac: e.getAttribute('autocomplete') || '', ph: e.getAttribute('placeholder') || '',
        al: e.getAttribute('aria-label') || '', txt: (e.innerText || '').trim().slice(0, 30),
      })))
    } catch (e) { inputs = [{ err: e.message.slice(0, 40) }] }
    if (inputs.length) {
      console.log(`--- frame: ${(f.url() || '(blank)').slice(0, 75)}`)
      inputs.forEach((i) => console.log('   ', JSON.stringify(i)))
    }
  }
  await browser.close()
})().catch((e) => console.error('ERR', e.message))
