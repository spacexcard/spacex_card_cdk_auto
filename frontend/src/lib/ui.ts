// 轻量交互助手：toast（DOM 注入）。confirm/prompt/alert 直接用 window.*（CDK 现有页面也这么用）。
let container: HTMLElement | null = null

function ensureContainer(): HTMLElement {
  if (container && document.body.contains(container)) return container
  container = document.createElement('div')
  container.style.cssText = 'position:fixed;top:16px;right:16px;z-index:9999;display:flex;flex-direction:column;gap:8px;pointer-events:none'
  document.body.appendChild(container)
  return container
}

export function toast(msg: string, type: 'ok' | 'err' | 'warn' | 'info' = 'ok', ms = 3000) {
  const el = document.createElement('div')
  const bg = type === 'err' ? '#dc2626' : type === 'warn' ? '#d97706' : type === 'info' ? '#2563eb' : '#059669'
  el.style.cssText = `background:${bg};color:#fff;padding:10px 16px;border-radius:8px;font-size:14px;box-shadow:0 4px 12px rgba(0,0,0,.2);max-width:420px;word-break:break-all;opacity:0;transition:opacity .2s;pointer-events:auto`
  el.textContent = msg
  ensureContainer().appendChild(el)
  requestAnimationFrame(() => { el.style.opacity = '1' })
  setTimeout(() => { el.style.opacity = '0'; setTimeout(() => el.remove(), 250) }, ms)
}
