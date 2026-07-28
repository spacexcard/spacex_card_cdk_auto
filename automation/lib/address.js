'use strict'

// 免税地址：默认本地生成（美国无销售税州：DE/OR/NH/MT/AK），100% 可靠；
// 也可用 source='usaddressgen' 去 https://usaddressgen.com/tax-free-address/ 抓取，失败自动回退本地。

function rand(arr) { return arr[Math.floor(Math.random() * arr.length)] }
function randInt(min, max) { return Math.floor(Math.random() * (max - min + 1)) + min }

const FIRST = ['James', 'John', 'Robert', 'Michael', 'David', 'William', 'Mary', 'Jennifer', 'Linda', 'Patricia', 'Elizabeth', 'Susan']
const LAST = ['Smith', 'Johnson', 'Williams', 'Brown', 'Jones', 'Garcia', 'Miller', 'Davis', 'Wilson', 'Anderson', 'Taylor', 'Moore']
const STREETS = ['Main St', 'Oak Ave', 'Maple Dr', 'Pine St', 'Cedar Ln', 'Elm St', 'Washington Ave', 'Park Rd', 'Lake Dr', 'Hill St']

// 无销售税州的真实城市 + 邮编前缀 + 区号
const TAX_FREE = [
  { state: 'DE', cities: [['Wilmington', '198'], ['Newark', '197'], ['Dover', '199']], area: '302' },
  { state: 'OR', cities: [['Portland', '972'], ['Salem', '973'], ['Eugene', '974']], area: '503' },
  { state: 'NH', cities: [['Manchester', '031'], ['Nashua', '030'], ['Concord', '033']], area: '603' },
  { state: 'MT', cities: [['Billings', '591'], ['Missoula', '598'], ['Bozeman', '597']], area: '406' },
  { state: 'AK', cities: [['Anchorage', '995'], ['Fairbanks', '997'], ['Juneau', '998']], area: '907' },
]

function localTaxFreeAddress() {
  const s = rand(TAX_FREE)
  const [city, zipPrefix] = rand(s.cities)
  const zip = zipPrefix + String(randInt(0, 99)).padStart(2, '0')
  const phone = `${s.area}${randInt(2000000, 9999999)}`
  return {
    firstName: rand(FIRST),
    lastName: rand(LAST),
    get name() { return `${this.firstName} ${this.lastName}` },
    line1: `${randInt(10, 9999)} ${rand(STREETS)}`,
    city,
    state: s.state,
    zip,
    country: 'US',
    phone,
    source: 'local',
  }
}

// 尝试从 usaddressgen 抓取（点“生成”按钮后读取结果文本并解析）。选择器未知，best-effort。
async function fetchFromUsAddressGen(browser) {
  let page
  try {
    page = await browser.newPage()
    await page.goto('https://usaddressgen.com/tax-free-address/', { waitUntil: 'domcontentloaded', timeout: 30000 })
    // 点击任何看起来像“生成/Generate”的按钮
    const btn = page.locator('button:has-text("生成"), button:has-text("Generate"), a:has-text("生成"), a:has-text("Generate")').first()
    if (await btn.count()) {
      await btn.click().catch(() => {})
      await page.waitForTimeout(1500)
    }
    const bodyText = await page.locator('body').innerText()
    await page.close().catch(() => {})

    // 解析：邮编 5 位 + 州两字母 + 城市 + 街道；电话
    const zipState = bodyText.match(/([A-Za-z][A-Za-z .'-]+),?\s*([A-Z]{2})\s+(\d{5})(?:-\d{4})?/)
    const phone = (bodyText.match(/\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}/) || [])[0]
    const line1 = (bodyText.match(/\d{1,5}\s+[A-Za-z0-9.'\- ]+(?:St|Ave|Dr|Rd|Ln|Blvd|Way|Ct|Street|Avenue|Drive|Road|Lane)\b/) || [])[0]
    if (zipState && line1) {
      return {
        firstName: rand(FIRST),
        lastName: rand(LAST),
        get name() { return `${this.firstName} ${this.lastName}` },
        line1: line1.trim(),
        city: zipState[1].trim(),
        state: zipState[2],
        zip: zipState[3],
        country: 'US',
        phone: (phone || '').replace(/\D/g, ''),
        source: 'usaddressgen',
      }
    }
  } catch (e) {
    if (page) await page.close().catch(() => {})
  }
  return null
}

async function getAddress(source, browser) {
  if (source === 'usaddressgen') {
    const a = await fetchFromUsAddressGen(browser)
    if (a) return a
    console.log('  [address] usaddressgen 抓取失败，回退本地生成')
  }
  return localTaxFreeAddress()
}

module.exports = { getAddress, localTaxFreeAddress }
