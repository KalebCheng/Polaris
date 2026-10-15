#!/usr/bin/env node
/**
 * 控制台截图脚本（开发自检用）。
 *
 * 用法：
 *   node scripts/screenshot.mjs <输出文件> <路径> [视口宽] [视口高]
 *
 * 原理：先经后端登录拿到会话 Cookie，再用 Chrome DevTools Protocol 注入 Cookie 后截图。
 * 依赖本机已安装 Chrome；Chrome 以无头模式临时启动。
 */
import { spawn } from 'node:child_process'
import { mkdtemp, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const CHROME = process.env.CHROME_PATH ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const FRONTEND = process.env.FRONTEND_URL ?? 'http://localhost:5173'
const BACKEND = process.env.BACKEND_URL ?? 'http://localhost:8100'
const INST = process.env.POLARIS_INSTITUTION ?? 'built-in'
const USER = process.env.POLARIS_USER ?? 'admin'
const PASS = process.env.POLARIS_PASSWORD ?? 'admin123'

const [, , outFile = '/tmp/polaris.png', route = '/', width = '1440', height = '900', theme = 'dark'] = process.argv

/** 登录并取回会话 Cookie 值 */
async function login() {
  const response = await fetch(`${BACKEND}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ institution: INST, username: USER, password: PASS }),
  })
  if (!response.ok) throw new Error(`登录失败: HTTP ${response.status}`)
  const raw = response.headers.getSetCookie?.() ?? []
  const cookie = raw.map((item) => item.split(';')[0]).find((item) => item.startsWith('polaris_session='))
  if (!cookie) throw new Error('登录响应未返回会话 Cookie')
  return { name: 'polaris_session', value: cookie.split('=').slice(1).join('=') }
}

async function connect() {
  const profile = await mkdtemp(join(tmpdir(), 'polaris-shot-'))
  const chrome = spawn(
    CHROME,
    [
      '--headless=new',
      '--disable-gpu',
      '--no-first-run',
      '--no-default-browser-check',
      '--remote-debugging-port=0',
      `--user-data-dir=${profile}`,
      'about:blank',
    ],
    { stdio: ['ignore', 'ignore', 'pipe'] },
  )

  const endpoint = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('等待 Chrome 调试端口超时')), 15000)
    chrome.stderr.on('data', (chunk) => {
      const match = /ws:\/\/[^\s]+/.exec(String(chunk))
      if (match) {
        clearTimeout(timer)
        resolve(match[0])
      }
    })
    chrome.on('exit', () => reject(new Error('Chrome 意外退出')))
  })

  const ws = new WebSocket(endpoint)
  await new Promise((resolve, reject) => {
    ws.onopen = resolve
    ws.onerror = () => reject(new Error('连接 CDP 失败'))
  })

  let id = 0
  const pending = new Map()
  ws.onmessage = (event) => {
    const message = JSON.parse(event.data)
    const waiter = pending.get(message.id)
    if (waiter) {
      pending.delete(message.id)
      message.error ? waiter.reject(new Error(message.error.message)) : waiter.resolve(message.result)
    }
  }
  const send = (method, params = {}, sessionId) =>
    new Promise((resolve, reject) => {
      const messageId = ++id
      pending.set(messageId, { resolve, reject })
      ws.send(JSON.stringify({ id: messageId, method, params, sessionId }))
    })

  return { chrome, ws, send }
}

const cookie = await login()
const { chrome, ws, send } = await connect()

try {
  const target = await send('Target.createTarget', { url: 'about:blank' })
  const attached = await send('Target.attachToTarget', { targetId: target.targetId, flatten: true })
  const session = attached.sessionId

  await send('Page.enable', {}, session)
  await send('Network.enable', {}, session)
  await send('Emulation.setDeviceMetricsOverride',
    { width: Number(width), height: Number(height), deviceScaleFactor: 2, mobile: false }, session)
  await send('Network.setCookie',
    { name: cookie.name, value: cookie.value, domain: 'localhost', path: '/', httpOnly: true }, session)
  // 主题由 next-themes 读写 localStorage，先落到目标站点再注入后重新加载
  await send('Page.navigate', { url: `${FRONTEND}${route}` }, session)
  await new Promise((resolve) => setTimeout(resolve, 800))
  await send('Runtime.evaluate',
    { expression: `window.localStorage.setItem('theme', ${JSON.stringify(theme)})` }, session)
  await send('Page.reload', {}, session)

  await new Promise((resolve) => setTimeout(resolve, 2500))

  // 可选：截图前执行一段 JS（例如点开对话框），便于检查交互态
  if (process.env.SCREENSHOT_EXPR) {
    await send('Runtime.evaluate', { expression: process.env.SCREENSHOT_EXPR, awaitPromise: true }, session)
    await new Promise((resolve) => setTimeout(resolve, 1200))
  }

  // 可选：截图后输出一段求值结果（用于断言交互结果，如跳转后的路径）
  if (process.env.SCREENSHOT_VERIFY_EXPR) {
    const result = await send('Runtime.evaluate', { expression: process.env.SCREENSHOT_VERIFY_EXPR, returnByValue: true }, session)
    console.log('VERIFY:', JSON.stringify(result.result?.value ?? null))
  }

  const shot = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false }, session)
  await writeFile(outFile, Buffer.from(shot.data, 'base64'))
  console.log(`已保存截图: ${outFile}`)
} finally {
  ws.close()
  chrome.kill()
}
