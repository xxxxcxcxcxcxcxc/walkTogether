import WebApp from '@twa-dev/sdk'

let webApp: any = null

export function initTelegramApp(): any {
  if (webApp) return webApp

  webApp = WebApp

  if (typeof webApp?.expand === 'function') {
    webApp.expand()
  }

  if (typeof webApp?.enableVerticalSwipes === 'function') {
    webApp.enableVerticalSwipes()
  }

  if (typeof webApp?.setHeaderColor === 'function') {
    webApp.setHeaderColor('#0ea5e9')
  }

  if (typeof webApp?.setBackgroundColor === 'function') {
    webApp.setBackgroundColor('#ffffff')
  }

  return webApp
}

export function getWebApp(): any {
  return webApp
}

export function getInitData(): string {
  return String(webApp?.initData ?? '')
}

export function getStartParam(): string {
  return String((webApp?.initDataUnsafe?.start_param ?? (typeof window !== 'undefined' ? (window as any).Telegram?.WebApp?.initDataUnsafe?.start_param : '')) ?? '')
}

export function getInitDataUnsafe(): Record<string, unknown> {
  return (webApp?.initDataUnsafe ?? {}) as Record<string, unknown>
}

export function isTelegramWebApp(): boolean {
  return !!webApp
}

export function isTelegramEnvironment(): boolean {
  return typeof window !== 'undefined' && !!(window as any).Telegram?.WebApp
}

export function ready(): void {
  webApp?.ready?.()
}

export function close(): void {
  webApp?.close?.()
}

export function hapticFeedback(type: 'light' | 'medium' | 'heavy' | 'success' | 'error' | 'warning' = 'light'): void {
  const impact = type === 'success' || type === 'error' || type === 'warning' ? type : type === 'light' ? 'soft' : type === 'medium' ? 'rigid' : 'heavy'
  webApp?.HapticFeedback?.impactOccurred?.(impact)
}

export function hapticNotification(type: 'success' | 'error' | 'warning'): void {
  webApp?.HapticFeedback?.notificationOccurred?.(type)
}

export function showAlert(message: string): Promise<void> {
  return new Promise((resolve) => {
    webApp?.showAlert?.(message, resolve)
  })
}

export function showConfirm(message: string): Promise<boolean> {
  return new Promise((resolve) => {
    webApp?.showConfirm?.(message, resolve)
  })
}

export function openLink(url: string): void {
  webApp?.openLink?.(url)
}

export function openTelegramLink(url: string): void {
  webApp?.openTelegramLink?.(url)
}

export function shareToStory(mediaUrl: string, params?: { text?: string; widget_link?: { url: string; name: string } }): void {
  webApp?.shareToStory?.(mediaUrl, params)
}

export function requestWriteAccess(): Promise<boolean> {
  return new Promise((resolve) => {
    webApp?.requestWriteAccess?.(resolve)
  })
}

export function requestContact(): Promise<void> {
  return new Promise((resolve) => {
    webApp?.requestContact?.(() => resolve())
  })
}

export function switchInlineQuery(query: string, chooseChatTypes?: string[]): void {
  webApp?.switchInlineQuery?.(query, chooseChatTypes as any)
}

export function setMainButton(params: { text: string; color?: string; textColor?: string; isVisible?: boolean; isActive?: boolean; onClick: () => void }): void {
  const btn = webApp?.MainButton
  if (btn) {
    btn.setText(params.text)
    if (params.color) btn.setParams({ color: params.color })
    if (params.textColor) btn.setParams({ text_color: params.textColor })
    btn.onClick(params.onClick)
    if (params.isVisible !== false) btn.show()
    else btn.hide()
    btn.enable()
  }
}

export function hideMainButton(): void {
  webApp?.MainButton?.hide?.()
}

export function setBackButton(onClick: () => void, isVisible: boolean = true): void {
  const btn = webApp?.BackButton
  if (btn) {
    btn.onClick(onClick)
    if (isVisible) btn.show()
    else btn.hide()
  }
}

export function hideBackButton(): void {
  webApp?.BackButton?.hide?.()
}

export function getThemeParams(): Record<string, any> {
  return webApp?.themeParams || {}
}

export function getColorScheme(): 'light' | 'dark' {
  return webApp?.colorScheme || 'light'
}

export function getViewportHeight(): number {
  return webApp?.viewportHeight || window.innerHeight
}

export function getViewportStableHeight(): number {
  return webApp?.viewportStableHeight || window.innerHeight
}

export function onViewportChange(callback: (height: number, isStateStable: boolean) => void): void {
  webApp?.onEvent?.('viewportChanged', (params: any) => callback(params?.viewportHeight ?? 0, !!params?.isStateStable))
}

export function offViewportChange(callback: (height: number, isStateStable: boolean) => void): void {
  webApp?.offEvent?.('viewportChanged', (params: any) => callback(params?.viewportHeight ?? 0, !!params?.isStateStable))
}

export function onThemeChange(callback: (themeParams: Record<string, any>) => void): void {
  webApp?.onEvent?.('themeChanged', (params: any) => callback(params || {}))
}

export function offThemeChange(callback: (themeParams: Record<string, any>) => void): void {
  webApp?.offEvent?.('themeChanged', (params: any) => callback(params || {}))
}