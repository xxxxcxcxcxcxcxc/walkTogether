import { render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const {
  getCurrentUserMock,
  loginWithTelegramMock,
  getInitDataMock,
  getStartParamMock,
  initTelegramAppMock,
  isTelegramEnvironmentMock,
} = vi.hoisted(() => ({
  getCurrentUserMock: vi.fn(),
  loginWithTelegramMock: vi.fn(),
  getInitDataMock: vi.fn(),
  getStartParamMock: vi.fn(),
  initTelegramAppMock: vi.fn(),
  isTelegramEnvironmentMock: vi.fn(),
}))

vi.mock('./services/auth', () => ({
  getCurrentUser: getCurrentUserMock,
  loginWithTelegram: loginWithTelegramMock,
}))

vi.mock('./services/telegram', () => ({
  getInitData: getInitDataMock,
  getStartParam: getStartParamMock,
  initTelegramApp: initTelegramAppMock,
  isTelegramEnvironment: isTelegramEnvironmentMock,
}))

vi.mock('./hooks/useTelegramTheme', () => ({
  useTelegramTheme: () => ({ bgColor: '#ffffff', isDark: false }),
}))

vi.mock('./components/BottomNavigation', () => ({ BottomNavigation: () => null }))
vi.mock('./pages/home/HomePage', () => ({ HomePage: () => null }))
vi.mock('./pages/availability/AvailabilityPage', () => ({ AvailabilityPage: () => null }))
vi.mock('./pages/walks/WalksPage', () => ({ WalksPage: () => null }))
vi.mock('./pages/friends/FriendsPage', () => ({ FriendsPage: () => null }))
vi.mock('./pages/profile/ProfilePage', () => ({ ProfilePage: () => null }))

import { App } from './App'

describe('App authentication bootstrap', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getCurrentUserMock.mockResolvedValue({
      id: 'user-1',
      telegram_id: 123,
      display_name: 'Telegram User',
      role: 'user',
      is_admin: false,
      is_active: true,
      is_deleted: false,
      created_at: '2026-01-01T00:00:00Z',
    })
  })

  it('does not send the invite token again when an existing session is valid', async () => {
    render(<App />)

    await waitFor(() => expect(getCurrentUserMock).toHaveBeenCalledTimes(1))

    expect(loginWithTelegramMock).not.toHaveBeenCalled()
    expect(getStartParamMock).not.toHaveBeenCalled()
  })
})