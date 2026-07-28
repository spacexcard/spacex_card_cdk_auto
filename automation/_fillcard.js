const { chromium } = require('playwright')
const PORT = process.argv[2] || '9336'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
;(async () => {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${PORT}`)
  const ctx = browser.contexts()[0]
  const page = ctx.pages().find((p) => p.url().includes('checkout')) || ctx.pages()[ctx.pages().length - 1]
  console.log('reload + 等待 Stripe 渲染...')
  await page.reload({ waitUntil: 'domcontentloaded' }).catch(() => {})

  // 轮询等待卡号框出现（最多 25s）
  let cardFrame = null
  for (let i = 0; i < 25 && !cardFrame; i++) {
    for (const f of page.frames()) {
      try { if (await f.locator('#payment-numberInput, input[name=number]').count()) { cardFrame = f; break } } catch (e) {}
    }
    if (!cardFrame) await sleep(1000)
  }
  if (!cardFrame) { console.log('25s 内仍未出现卡号框'); await browser.close(); return }
  console.log('✓ 找到卡号 frame:', cardFrame.url().slice(0, 55))

  const type = async (label, sel, val) => {
    try {
      const loc = cardFrame.locator(sel).first()
      await loc.click({ timeout: 3000 })
      await loc.pressSequentially(val, { delay: 45 })
      console.log('  ✓', label)
    } catch (e) { console.log('  ✗', label, e.message.slice(0, 50)) }
  }
  await type('卡号', '#payment-numberInput', '4242424242424242')
  await type('有效期', '#payment-expiryInput', '1230')
  await type('CVC', '#payment-cvcInput', '123')

  console.log('\n=== 账单地址字段 ===')
  for (const f of page.frames()) {
    try {
      const a = await f.$$eval('input,select', (els) => els.map((e) => ({ name: e.name, id: e.id, ac: e.getAttribute('autocomplete') || '', ph: e.getAttribute('placeholder') || '', al: e.getAttribute('aria-label') || '' }))
        .filter((x) => /address|postal|zip|city|state|country|line|name|locality|administrative/i.test(JSON.stringify(x))))
      if (a.length) { console.log('--- frame', (f.url() || '(blank)').slice(0, 55)); a.forEach((x) => console.log('   ', JSON.stringify(x))) }
    } catch (e) {}
  }
  console.log('\n请看浏览器：卡号/有效期/CVC 是否已填上？')
  await browser.close()
})().catch((e) => console.error('ERR', e.message))
