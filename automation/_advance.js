// 在已打开的结账页点 "Subscribe" 进入填卡步骤，然后 dump 所有 frame 的输入框（找 Stripe 卡号字段）
const { chromium } = require('playwright')
const PORT = process.argv[2] || '9336'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
;(async () => {
  const browser = await chromium.connectOverCDP(`http://127.0.0.1:${PORT}`)
  const ctx = browser.contexts()[0]
  const page = ctx.pages().find((p) => p.url().includes('checkout')) || ctx.pages()[ctx.pages().length - 1]
  console.log('URL:', page.url())

  // 确保 20x(chatgptpro) 选中
  try { await page.locator('#chatgptpro').click({ timeout: 2000 }); console.log('点了 20x 卡片') } catch (e) {}
  await sleep(500)
  // 点 Subscribe 进入下一步
  try {
    await page.locator('button[aria-label="Subscribe"], button:has-text("Subscribe")').first().click({ timeout: 3000 })
    console.log('已点 Subscribe，等待填卡表单...')
  } catch (e) { console.log('点 Subscribe 失败:', e.message) }
  await sleep(7000)

  console.log('\n=== 点击后各 frame 的输入框 ===')
  for (const f of page.frames()) {
    let inputs = []
    try {
      inputs = await f.$$eval('input,select', (els) => els.map((e) => ({
        name: e.name || '', type: e.type || '', id: e.id || '',
        ac: e.getAttribute('autocomplete') || '', ph: e.getAttribute('placeholder') || '', al: e.getAttribute('aria-label') || '',
      })))
    } catch (e) {}
    if (inputs.length) {
      console.log(`--- frame: ${(f.url() || '(blank)').slice(0, 72)}`)
      inputs.forEach((i) => console.log('   ', JSON.stringify(i)))
    }
  }
  await browser.close()
})().catch((e) => console.error('ERR', e.message))
