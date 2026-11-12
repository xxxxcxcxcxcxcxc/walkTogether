import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@twa-dev/sdk', () => ({
  default: {
    initData: 'signed-telegram-init-data',
    initDataUnsafe: { start_param: 'telegram-startapp-token' },
    expand: vi.fn(),
    enableVerticalSwipes: vi.fn(),
    setHeaderColor: vi.fn(),
    setBackgroundColor: vi.fn(),
  },
}))

import { getStartParam, initTelegramApp } from './telegram'

describe('Telegram start parameter', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/?startapp=untrusted-url-value')
  })

  it('reads start_param from the Telegram SDK instead of arbitrary URL parameters', () => {
    initTelegramApp()

    expect(getStartParam()).toBe('telegram-startapp-token')
  })
})