import '@testing-library/jest-dom/vitest'

// jsdom 未实现 matchMedia / ResizeObserver，主题与 Radix 组件需要
if (!window.matchMedia) {
  window.matchMedia = (query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
}

// jsdom 未实现 elementFromPoint（input-otp 用它做光标定位，纯测量用途）。
// 不补这个桩的话，动态码组件卸载后的定时器会抛未捕获异常，让 vitest 以非 0 退出。
if (!document.elementFromPoint) {
  document.elementFromPoint = () => null
}

// jsdom 未实现 scrollIntoView（快速跳转面板用"键盘移动时把高亮项滚进视野"）。
// 纯测量用途，补空实现即可——否则该面板一渲染就抛 TypeError。
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {}
}
