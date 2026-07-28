'use strict'

/*
 * ChatGPT 充值支付页自动填表（Playwright / Chrome）
 *
 * 架构（关键）：生成支付链接交给【后端 tls-client】（能过 Cloudflare），
 *   浏览器只负责打开 pay.openai.com 并【自动填卡号+地址】。
 *   —— 因为 Playwright 操控的浏览器在 chatgpt.com 会被 Cloudflare 判定为机器人(403)，
 *      所以不在浏览器里生成链接。
 *
 * 不自动提交（除非 --submit）；遇到 3DS/验证码，浏览器保持打开由你人工完成。
 *
 * 用法：
 *   # 用 session 文件（后端据此生成链接，需后端在 :8080 运行）
 *   node fill-checkout.js --session /path/to/session --plan ph-20x
 *   # 只测填表：直接给已生成的支付链接（最快，不依赖后端/session）
 *   node fill-checkout.js --pay-url "https://pay.openai.com/c/pay/cs_live_..."
 *
 * 其它参数：--backend http://localhost:8080  --admin-user admin  --admin-pass ****
 *           --address-source local|usaddressgen  --card 4242...  --country US  --submit  --headless
 */

const { chromium } = require('playwright')
const { spawn } = require('child_process')
const fs = require('fs')
const os = require('os')
const path = require('path')
const { randomCard } = require('./lib/card')
const { getAddress } = require('./lib/address')
const { buildSessionCookies } = require('./lib/session')

// 直接拉起真实 Chrome（不经 Playwright 启动器 → 无 webdriver 标记），全新临时配置=无痕
function findChromeBinary() {
  const cands = ['/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/opt/google/chrome/chrome',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']
  for (const c of cands) { try { if (fs.existsSync(c)) return c } catch (e) {} }
  return 'google-chrome'
}
async function waitForCdp(port, timeoutMs = 25000) {
  const start = Date.now()
  while (Date.now() - start < timeoutMs) {
    try { const r = await fetch(`http://127.0.0.1:${port}/json/version`); if (r.ok) return } catch (e) {}
    await new Promise((r) => setTimeout(r, 300))
  }
  throw new Error('Chrome 远程调试端口未就绪')
}
async function spawnRealChrome(port) {
  const userDir = fs.mkdtempSync(path.join(os.tmpdir(), 'cgpt-pay-'))
  const bin = findChromeBinary()
  const proc = spawn(bin, [
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${userDir}`,
    '--no-first-run', '--no-default-browser-check', '--disable-popup-blocking',
    '--new-window', 'about:blank',
  ], { stdio: 'ignore', detached: false })
  await waitForCdp(port)
  return { proc, userDir, bin }
}

const PLANS = {
  'ph-20x': { plan_name: 'chatgptpro', country: 'PH', currency: 'PHP' },
  'eg-5x': { plan_name: 'chatgptprolite', country: 'EG', currency: 'EGP' },
}

function parseArgs(argv) {
  const a = {
    plan: 'ph-20x', addressSource: 'local', submit: false, headless: false,
    backend: 'http://localhost:8080', adminUser: 'admin', adminPass: 'admin123456',
  }
  for (let i = 2; i < argv.length; i++) {
    const k = argv[i]; const next = () => argv[++i]
    if (k === '--session') a.session = next()
    else if (k === '--plan') a.plan = next()
    else if (k === '--pay-url') a.payUrl = next()
    else if (k === '--backend') a.backend = next()
    else if (k === '--admin-user') a.adminUser = next()
    else if (k === '--admin-pass') a.adminPass = next()
    else if (k === '--admin-token') a.adminToken = next()
    else if (k === '--address-source') a.addressSource = next()
    else if (k === '--card') a.card = next()
    else if (k === '--country') a.country = next()
    else if (k === '--submit') a.submit = true
    else if (k === '--headless') a.headless = true
    else if (k === '--pw-launch') a.pwLaunch = true
    else if (k === '--cdp') {
      // 连接你手动开的真实 Chrome（带 --remote-debugging-port）。可跟自定义地址。
      const v = argv[i + 1]
      a.cdp = v && !v.startsWith('--') ? (i++, v) : 'http://127.0.0.1:9222'
    }
  }
  return a
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// —— 通过后端(tls-client)生成支付链接 ——
async function backendLogin(backend, user, pass) {
  const r = await fetch(`${backend}/api/v1/auth/admin/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: user, password: pass }),
  })
  const d = await r.json().catch(() => ({}))
  if (!r.ok || !d.token) throw new Error(`后端登录失败 HTTP ${r.status}: ${d.error || ''}（检查后端是否运行、管理员密码是否正确）`)
  return d.token
}

async function generateViaBackend(args, sessionText) {
  const token = args.adminToken || (await backendLogin(args.backend, args.adminUser, args.adminPass))
  const p = PLANS[args.plan]
  if (!p) throw new Error(`未知套餐: ${args.plan}（可用 ${Object.keys(PLANS).join(' / ')}）`)
  const r = await fetch(`${args.backend}/api/v1/admin/checkout/generate`, {
    method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
    body: JSON.stringify({ token_input: sessionText, plan_name: p.plan_name, country: p.country, currency: p.currency }),
  })
  const d = await r.json().catch(() => ({}))
  if (!r.ok || !d.link) throw new Error(`生成支付链接失败 HTTP ${r.status}: ${JSON.stringify(d.error || d).slice(0, 300)}`)
  return d.link
}

// —— Stripe 支付页填表（跨主页面与 iframe 兜底匹配）——
const norm = (s) => String(s || '').replace(/\s/g, '').toLowerCase()

async function fillField(page, selectors, value, opts = {}) {
  const scopes = [page, ...page.frames().filter((f) => f !== page.mainFrame())]
  for (const scope of scopes) {
    for (const sel of selectors) {
      try {
        const loc = scope.locator(sel).first()
        if (!(await loc.count())) continue
        await loc.scrollIntoViewIfNeeded({ timeout: 1500 }).catch(() => {})
        if (opts.select) { await loc.selectOption(value, { timeout: 2000 }); return true }
        if (opts.type) {
          await loc.click({ timeout: 1500 }).catch(() => {})
          await loc.fill('', { timeout: 1500 }).catch(() => {})
          await loc.pressSequentially(value, { delay: 35, timeout: 5000 })
          return true
        }
        // 普通文本：fill 后按 Esc 关掉地址自动完成下拉，再校验；被截断/改写则逐字慢打重填
        await loc.fill(value, { timeout: 3000 })
        await loc.press('Escape').catch(() => {})
        await sleep(200)
        const cur = await loc.inputValue().catch(() => value)
        if (norm(cur) !== norm(value)) {
          await loc.fill('', { timeout: 1500 }).catch(() => {})
          await loc.pressSequentially(value, { delay: 70, timeout: 6000 }).catch(() => {})
          await loc.press('Escape').catch(() => {})
          await sleep(150)
        }
        return true
      } catch (e) { /* 试下一个 */ }
    }
  }
  return false
}
async function fillRegion(page, selectors, code) {
  if (await fillField(page, selectors, code, { select: true })) return true
  return fillField(page, selectors, code)
}

async function fillCheckout(page, { card, address, country, email }) {
  const filled = []
  const log = (label, ok) => filled.push(`${ok ? '✓' : '✗'} ${label}`)

  // 卡片（OpenAI 原生结账内嵌的 Stripe Payment Element，字段在 js.stripe.com iframe 里）
  log('卡号', await fillField(page, ['#payment-numberInput', 'input[name="number"]', 'input[autocomplete="cc-number"]'], card.number, { type: true }))
  log('有效期', await fillField(page, ['#payment-expiryInput', 'input[name="expiry"]', 'input[autocomplete="cc-exp"]'], card.month + card.year, { type: true }))
  log('CVC', await fillField(page, ['#payment-cvcInput', 'input[name="cvc"]', 'input[autocomplete="cc-csc"]'], card.cvc, { type: true }))

  // 账单地址（Stripe Address Element）
  log('姓名', await fillField(page, ['#billingAddress-nameInput', 'input[name="name"]', 'input[autocomplete="billing name"]'], address.name))
  if (country) log('国家', await fillRegion(page, ['#billingAddress-countryInput', 'select[name="country"]', 'select[autocomplete="billing country"]'], country))
  log('街道', await fillField(page, ['#billingAddress-addressLine1Input', 'input[name="addressLine1"]', 'input[autocomplete="billing address-line1"]'], address.line1))
  log('城市', await fillField(page, ['#billingAddress-localityInput', 'input[name="locality"]', 'input[autocomplete="billing address-level2"]'], address.city))
  log('州', await fillRegion(page, ['#billingAddress-administrativeAreaInput', 'select[name="administrativeArea"]', 'input[name="administrativeArea"]', 'select[autocomplete="billing address-level1"]', 'input[autocomplete="billing address-level1"]'], address.state))
  log('邮编', await fillField(page, ['#billingAddress-postalCodeInput', 'input[name="postalCode"]', 'input[autocomplete="billing postal-code"]'], address.zip))
  // 注意：不填 Stripe Link 的 email/phone（会触发 Link 验证）
  return filled
}

// 调试用：dump 输入框 + 按钮 + radio标签 + 关键词可点击项 + iframe，看清"已存卡/新增卡/地址"结构
async function inspectPage(page) {
  const scopes = [page, ...page.frames().filter((f) => f !== page.mainFrame())]
  const inputs = []
  for (const s of scopes) {
    try {
      const fields = await s.$$eval('input,select', (els) => els.map((e) => ({
        tag: e.tagName.toLowerCase(), id: e.id || '', name: e.name || '', type: e.type || '',
        ac: e.getAttribute('autocomplete') || '', ph: e.getAttribute('placeholder') || '',
      })))
      inputs.push(...fields)
    } catch (e) {}
  }
  let buttons = [], radios = [], clickable = []
  try {
    buttons = [...new Set(await page.$$eval('button,[role=button]', (els) => els.map((e) => (e.innerText || e.getAttribute('aria-label') || '').trim()).filter(Boolean)))]
    radios = await page.$$eval('input[type=radio]', (els) => els.map((e) => {
      let t = '', n = e
      for (let i = 0; i < 4 && n; i++) { n = n.parentElement; if (n && n.innerText) { t = n.innerText.trim(); break } }
      return { value: e.value, checked: e.checked, label: t.slice(0, 80) }
    }))
    clickable = [...new Set(await page.$$eval('a,button,label,[role=button],[role=radio]', (els) =>
      els.map((e) => (e.innerText || '').trim()).filter((t) => t && /card|address|卡|地址|add|新增|payment|支付|方式|new/i.test(t))))]
  } catch (e) {}
  return { inputs, buttons, radios, clickable }
}

// 若卡号框未出现，尝试点开"新增卡/添加支付方式"（账号已绑卡时会先显示已存卡的单选）
async function revealNewCard(page) {
  const texts = ['Add a new card', 'Add new card', 'New card', 'Add payment method', 'Use a different card',
    'Add a card', '新增卡', '添加卡', '使用新卡', '添加支付方式', '新的支付方式', '其他卡', 'New payment method']
  for (const t of texts) {
    try {
      const el = page.locator(`text=${t}`).first()
      if (await el.count()) { await el.click({ timeout: 2000 }).catch(() => {}); await sleep(1200); return t }
    } catch (e) {}
  }
  // 兜底：若有多个 radio，点最后一个（通常"新增"在已存卡下面）
  try {
    const rs = page.locator('input[type=radio]')
    const n = await rs.count()
    if (n > 1) { await rs.nth(n - 1).click({ timeout: 2000 }).catch(() => {}); await sleep(1200); return `radio#${n - 1}` }
  } catch (e) {}
  return ''
}

async function main() {
  const args = parseArgs(process.argv)

  let payUrl = args.payUrl
  let sessionCookies = []
  if (!payUrl) {
    if (!args.session) { console.error('需要 --session <session文件> 或 --pay-url <支付链接>'); process.exit(1) }
    const sessionText = fs.readFileSync(path.resolve(args.session), 'utf8').trim()
    // 解析 sessionToken 用于注入 cookie 登录（原生结账页需要登录态）
    try {
      const s = JSON.parse(sessionText)
      if (s.sessionToken) sessionCookies = buildSessionCookies(s.sessionToken)
    } catch (e) { /* 文件是裸 accessToken，则无法 cookie 登录 */ }
    console.log(`→ 通过后端(${args.backend}) tls-client 生成「${args.plan}」支付链接...`)
    payUrl = await generateViaBackend(args, sessionText)
    console.log('  支付链接:', payUrl)
  }

  let browser, context, spawned = null
  if (args.cdp) {
    // 接管你手动启动的真实 Chrome
    console.log(`→ 连接真实 Chrome (CDP ${args.cdp}) ...`)
    browser = await chromium.connectOverCDP(args.cdp)
    context = browser.contexts()[0] || (await browser.newContext())
  } else if (args.pwLaunch) {
    // 兜底：Playwright 启动（会被 CF 识别，仅供调试）
    browser = await chromium.launch({ channel: 'chrome', headless: args.headless })
    context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
  } else {
    // 默认：自动拉起全新无痕真实 Chrome，再 CDP 接管（无 webdriver 标记，CF 当真人）
    const port = 9222 + (process.pid % 900)
    console.log(`→ 自动启动全新无痕真实 Chrome (端口 ${port}) ...`)
    spawned = await spawnRealChrome(port)
    process.on('SIGINT', () => {
      try { spawned.proc.kill() } catch (e) {}
      try { fs.rmSync(spawned.userDir, { recursive: true, force: true }) } catch (e) {}
      process.exit(0)
    })
    browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`)
    context = browser.contexts()[0] || (await browser.newContext())
  }

  const card = args.card ? { number: args.card, month: '12', year: '30', cvc: '123' } : randomCard()
  const address = await getAddress(args.addressSource, browser)
  const billingCountry = args.country || address.country || 'US'

  // 注入 session cookie 登录该账号（原生结账页未登录会被重定向走）
  if (sessionCookies.length) {
    try {
      await context.addCookies(sessionCookies)
      console.log('→ 已注入 session cookie，预热 chatgpt.com 建立登录态...')
      const warm = await context.newPage()
      await warm.goto('https://chatgpt.com/', { waitUntil: 'domcontentloaded', timeout: 60000 }).catch(() => {})
      await sleep(5000) // 给 Cloudflare 自动放行 + 登录态建立时间
      await warm.close().catch(() => {})
    } catch (e) { console.log('  注入 cookie 失败:', e.message) }
  } else {
    console.log('  ⚠ 未取到 sessionToken（文件里只有 accessToken？），浏览器将是未登录态，原生结账页可能打不开')
  }

  const page = await context.newPage()
  console.log('→ 打开支付页:', payUrl)
  await page.goto(payUrl, { waitUntil: 'domcontentloaded', timeout: 60000 })

  // 确保选中 20x（chatgptpro）套餐
  if (args.plan === 'ph-20x') { await page.locator('#chatgptpro').click({ timeout: 3000 }).catch(() => {}) }
  else if (args.plan === 'eg-5x') { await page.locator('#chatgptprolite').click({ timeout: 3000 }).catch(() => {}) }

  // 轮询等待 Stripe Payment Element 渲染出卡号框（约需 10~20s）
  console.log('→ 等待 Stripe 卡号框渲染（最多 25s）...')
  let cardReady = false
  for (let i = 0; i < 25 && !cardReady; i++) {
    for (const f of page.frames()) {
      try { if (await f.locator('#payment-numberInput, input[name="number"]').count()) { cardReady = true; break } } catch (e) {}
    }
    if (!cardReady) await sleep(1000)
  }
  console.log(cardReady ? '  ✓ 卡号框已出现' : '  ⚠ 25s 内未出现卡号框（可能账号已绑卡，显示的是已存卡）')

  console.log('→ 自动填表中...')
  console.log('  卡号:', card.number, '| 有效期:', card.exp, '| CVC:', card.cvc)
  console.log('  地址:', `${address.name}, ${address.line1}, ${address.city}, ${address.state} ${address.zip} (${address.source})`)
  const result = await fillCheckout(page, { card, address, country: billingCountry })
  console.log('→ 填表结果:')
  result.forEach((r) => console.log('   ' + r))

  if (args.submit) {
    console.log('→ 点击支付（如弹 3DS/验证，请在浏览器里人工完成）...')
    const pay = page.locator('button[type=submit], .SubmitButton, button:has-text("Pay"), button:has-text("支付")').first()
    await pay.click({ timeout: 5000 }).catch(() => console.log('  未找到支付按钮，请人工点击'))
  }

  console.log('\n✅ 已完成自动填表。浏览器保持打开 —— 请核对、处理验证并完成支付。完成后关闭窗口或按 Ctrl+C 退出。')
  await page.waitForEvent('close', { timeout: 0 }).catch(() => {})
  await browser.close().catch(() => {})
  // 自动拉起的无痕 Chrome：跑完清理进程与临时配置
  if (spawned) {
    try { spawned.proc.kill() } catch (e) {}
    try { fs.rmSync(spawned.userDir, { recursive: true, force: true }) } catch (e) {}
  }
}

main().catch((e) => { console.error('✗ 出错:', e.message || e); process.exit(1) })
