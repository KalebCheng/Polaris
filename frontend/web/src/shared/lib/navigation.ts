/** 整页跳转（用于必须离开 SPA 的 OIDC / 外部 IdP 流程）。 */
export function navigateDocument(url: string): void {
  window.location.assign(url)
}
