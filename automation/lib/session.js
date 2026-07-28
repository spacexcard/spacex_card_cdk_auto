'use strict'

const fs = require('fs')

// NextAuth cookie 切片大小（与 session_to_cookie.py 一致）
const CHUNK_SIZE = 3933
const BASE = '__Secure-next-auth.session-token'

function parseSession(file) {
  const raw = fs.readFileSync(file, 'utf8').trim()
  const data = JSON.parse(raw)
  return {
    accessToken: (data.accessToken || '').trim(),
    sessionToken: (data.sessionToken || '').trim(),
    email: (data.user && data.user.email) || '',
    raw: data,
  }
}

// 把 sessionToken 切片成 Playwright 可用的 cookie 数组（注入后即为登录态）
function buildSessionCookies(sessionToken) {
  const parts = []
  if (!sessionToken) return parts
  const mk = (name, value) => ({
    name,
    value,
    domain: '.chatgpt.com',
    path: '/',
    httpOnly: true,
    secure: true,
    sameSite: 'Lax',
  })
  if (sessionToken.length <= CHUNK_SIZE) {
    parts.push(mk(BASE, sessionToken))
  } else {
    let idx = 0
    for (let i = 0; i < sessionToken.length; i += CHUNK_SIZE, idx++) {
      parts.push(mk(`${BASE}.${idx}`, sessionToken.slice(i, i + CHUNK_SIZE)))
    }
  }
  return parts
}

module.exports = { parseSession, buildSessionCookies }
