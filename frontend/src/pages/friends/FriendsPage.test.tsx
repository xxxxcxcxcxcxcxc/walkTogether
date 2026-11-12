import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const {
  createFriendRequestMock,
  getFriendPrivacySettingsMock,
  getFriendProfileMock,
  getFriendRequestsMock,
  getFriendsMock,
  getInvitationsMock,
  removeFriendMock,
  respondToFriendRequestMock,
  revokeInvitationMock,
  searchFriendsUsersMock,
  updateFriendPrivacySettingsMock,
} = vi.hoisted(() => ({
  createFriendRequestMock: vi.fn(),
  getFriendPrivacySettingsMock: vi.fn(),
  getFriendProfileMock: vi.fn(),
  getFriendRequestsMock: vi.fn(),
  getFriendsMock: vi.fn(),
  getInvitationsMock: vi.fn(),
  removeFriendMock: vi.fn(),
  respondToFriendRequestMock: vi.fn(),
  revokeInvitationMock: vi.fn(),
  searchFriendsUsersMock: vi.fn(),
  updateFriendPrivacySettingsMock: vi.fn(),
}))

vi.mock('../../hooks/useTelegramTheme', () => ({
  useTelegramTheme: () => ({
    bgColor: '#ffffff',
    textColor: '#111111',
    hintColor: '#666666',
    secondaryBgColor: '#f4f4f4',
  }),
}))

vi.mock('../../services/auth', () => ({
  createFriendRequest: createFriendRequestMock,
  createInvitation: vi.fn(),
  getFriendPrivacySettings: getFriendPrivacySettingsMock,
  getFriendProfile: getFriendProfileMock,
  getFriendRequests: getFriendRequestsMock,
  getFriends: getFriendsMock,
  getInvitations: getInvitationsMock,
  removeFriend: removeFriendMock,
  respondToFriendRequest: respondToFriendRequestMock,
  revokeInvitation: revokeInvitationMock,
  searchFriendsUsers: searchFriendsUsersMock,
  updateFriendPrivacySettings: updateFriendPrivacySettingsMock,
}))

import { FriendsPage } from './FriendsPage'

afterEach(() => cleanup())

describe('FriendsPage invitation loading', () => {
  beforeEach(() => {
    getFriendsMock.mockResolvedValue([])
    getFriendRequestsMock.mockResolvedValue([])
    getFriendPrivacySettingsMock.mockResolvedValue({ visibility: 'all', friend_notifications_enabled: true })
    getInvitationsMock.mockResolvedValue({ status: 'success', slots_used: 0, slots_total: 5, items: [] })
    searchFriendsUsersMock.mockResolvedValue([])
    vi.clearAllMocks()
  })

  it('shows an API error instead of the empty state and retries successfully', async () => {
    getInvitationsMock
      .mockRejectedValueOnce(new Error('Сервер временно недоступен'))
      .mockResolvedValueOnce({ status: 'success', slots_used: 0, slots_total: 5, items: [] })

    render(<FriendsPage />)
    fireEvent.click(screen.getByRole('button', { name: /Приглашения/ }))

    expect((await screen.findByRole('alert')).textContent).toContain('Сервер временно недоступен')
    expect(screen.queryByText('Приглашений пока нет')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))

    expect(await screen.findByText('Приглашений пока нет')).toBeTruthy()
    expect(getInvitationsMock).toHaveBeenCalledTimes(2)
  })

  it('renders friends from the friends API and opens a privacy-filtered profile', async () => {
    const friend = {
      public_id: 'public-friend-1',
      display_name: 'Ирина Волкова',
      username: 'irina',
      joined_at: '2025-03-04T00:00:00Z',
      relationship_status: 'accepted' as const,
    }
    getFriendsMock.mockResolvedValue([friend])
    getFriendProfileMock.mockResolvedValue(friend)

    render(<FriendsPage />)

    fireEvent.click(await screen.findByText('Ирина Волкова'))
    expect(await screen.findByRole('dialog')).toBeTruthy()
    expect(screen.getByText('Вы друзья')).toBeTruthy()
    expect(getFriendProfileMock).toHaveBeenCalledWith('public-friend-1')
  })

  it('debounces directory search and creates a request for an unconnected user', async () => {
    searchFriendsUsersMock.mockResolvedValue([{
      public_id: 'public-user-2',
      display_name: 'Мария Иванова',
      username: 'maria',
      joined_at: '2025-04-05T00:00:00Z',
      relationship_status: 'none',
    }])
    createFriendRequestMock.mockResolvedValue({ id: 'request-1', status: 'pending' })

    render(<FriendsPage />)
    fireEvent.change(screen.getByRole('searchbox', { name: 'Найти участника' }), { target: { value: 'ma' } })

    const addButton = await screen.findByRole('button', { name: 'Добавить' })
    expect(searchFriendsUsersMock).toHaveBeenCalledWith('ma')
    fireEvent.click(addButton)
    expect(await screen.findByRole('status')).toBeTruthy()
    expect(createFriendRequestMock).toHaveBeenCalledWith('public-user-2')
  })

  it('accepts an incoming request and saves privacy settings', async () => {
    const incomingRequest = {
      id: 'request-2',
      status: 'pending' as const,
      created_at: '2025-05-06T00:00:00Z',
      user: {
        public_id: 'public-user-3',
        display_name: 'Олег Смирнов',
        joined_at: '2025-05-01T00:00:00Z',
        relationship_status: 'incoming_pending' as const,
        request_id: 'request-2',
      },
    }
    getFriendRequestsMock.mockImplementation((direction: string) => Promise.resolve(direction === 'incoming' ? [incomingRequest] : []))
    respondToFriendRequestMock.mockResolvedValue(undefined)
    updateFriendPrivacySettingsMock.mockImplementation((settings) => Promise.resolve(settings))

    render(<FriendsPage />)
    fireEvent.click(screen.getByRole('button', { name: /Запросы/ }))
    fireEvent.click(await screen.findByRole('button', { name: 'Принять запрос от Олег Смирнов' }))

    expect(respondToFriendRequestMock).toHaveBeenCalledWith('request-2', 'accept')
    fireEvent.click(screen.getByRole('button', { name: /Друзья/ }))
    fireEvent.change(await screen.findByLabelText('Кто может найти профиль'), { target: { value: 'friends' } })
    expect(updateFriendPrivacySettingsMock).toHaveBeenCalledWith({ visibility: 'friends', friend_notifications_enabled: true })
  })
})