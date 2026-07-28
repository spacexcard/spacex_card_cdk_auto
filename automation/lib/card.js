'use strict'

// 生成随机但 Luhn 合法的测试卡号。测试阶段只为验证“能否自动填表”，
// live 支付会被拒属正常；后期接 cardplatform 时替换 randomCard 即可。

function luhnComplete(partial) {
  // partial 不含校验位；返回带校验位的完整号
  const digits = partial.split('').map(Number)
  let sum = 0
  let alt = true // 从右往左（即将追加校验位），倒数第二位起 double
  for (let i = digits.length - 1; i >= 0; i--) {
    let d = digits[i]
    if (alt) {
      d *= 2
      if (d > 9) d -= 9
    }
    sum += d
    alt = !alt
  }
  const check = (10 - (sum % 10)) % 10
  return partial + String(check)
}

function randInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min
}

function randomCard() {
  // Visa：'4' 开头，共 16 位（15 位随机 + 1 位校验）
  let body = '4'
  for (let i = 0; i < 14; i++) body += randInt(0, 9)
  const number = luhnComplete(body)

  const now = new Date()
  const month = String(randInt(1, 12)).padStart(2, '0')
  const year = String(now.getFullYear() + randInt(1, 4)).slice(-2)
  const cvc = String(randInt(0, 999)).padStart(3, '0')

  return { number, exp: `${month}/${year}`, month, year, cvc }
}

module.exports = { randomCard, luhnComplete }
