export type AuthUser = {
  id: string
  telegram_id: number
  display_name: string
  username?: string
  avatar_url?: string
  role: string
  is_admin: boolean
  is_active: boolean
  is_deleted: boolean
  created_at: string
}

export type NotificationSettings = { friend_notifications_enabled: boolean; walk_notifications_enabled: boolean }

export type ApiAuthResult = {
  ok: boolean
  user?: AuthUser
  code?: string
  message?: string
}

export type InvitationItem = {
  id: string
  sender_user_id: string
  recipient_user_id?: string
  status: 'pending' | 'accepted' | 'rejected' | 'revoked' | 'expired'
  deep_link?: string
  expires_at?: string
  created_at?: string
  updated_at?: string
  accepted_at?: string
  revoked_at?: string
}

export type InvitationList = {
  status: string
  slots_used: number
  slots_total: number
  items: InvitationItem[]
}

export type FriendRelationshipStatus = 'none' | 'outgoing_pending' | 'incoming_pending' | 'accepted' | 'rejected' | 'cancelled'

export type FriendProfile = {
  public_id: string
  display_name: string
  username?: string
  avatar_url?: string
  joined_at: string
  relationship_status: FriendRelationshipStatus
  request_id?: string
}

export type FriendRequestItem = {
  id: string
  status: 'pending'
  created_at: string
  user: FriendProfile
}

export type FriendPrivacySettings = {
  visibility: 'all' | 'friends' | 'hidden'
  friend_notifications_enabled: boolean
}

export type AvailabilityInterval = {
  id?: string
  start_time: string
  end_time: string
  kind: 'free' | 'busy'
}

export type AvailabilityDay = {
  date: string
  state: 'unknown' | 'free_all' | 'busy_all' | 'custom'
  intervals: AvailabilityInterval[]
}

export type AvailabilityRule = {
  id: string
  day_of_week: number
  start_time: string
  end_time: string
  kind: 'free' | 'busy'
  starts_on: string
  ends_on?: string | null
  timezone: string
  is_active: boolean
}

export type FriendAvailability = {
  public_id: string
  display_name: string
  date: string
  state: 'unknown' | 'free' | 'busy' | 'free_all' | 'busy_all' | 'custom'
  exact: boolean
  intervals?: AvailabilityInterval[]
  timezone?: string
}

export type WalkParticipant = { public_id: string; display_name: string; status: string; is_required: boolean; responded_at?: string }
export type WalkHistoryItem = { event_type: string; actor_name?: string; details: Record<string, unknown>; created_at: string }
export type WalkEvent = {
  id: string; title: string; description?: string; status: 'draft' | 'pending' | 'confirmed' | 'rejected' | 'cancelled' | 'expired' | 'completed' | 'rescheduled'
  scheduled_start: string; scheduled_end: string; timezone: string; location_name?: string; latitude?: number; longitude?: number
  organizer_public_id: string; organizer_name: string; participants: WalkParticipant[]; history: WalkHistoryItem[]; is_organizer: boolean
}
export type WalkTimeSlot = { start: string; end: string }
export type FavoritePlace = { id: string; name: string; address: string; latitude: number; longitude: number }
export type PlaceSuggestion = Omit<FavoritePlace, 'id'>

const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? ''

async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
    ...options,
  })

  const text = await response.text()
  const data = text ? JSON.parse(text) : null

  if (!response.ok) {
    throw new Error(data?.message ?? `request failed: ${response.status}`)
  }

  return data as T
}

export async function getCurrentUser(): Promise<AuthUser | null> {
  try {
    const payload = await apiFetch<{ user?: AuthUser }>('/api/me', { method: 'GET' })
    return payload.user ?? null
  } catch {
    return null
  }
}

export async function loginWithTelegram(initData: string, inviteToken?: string): Promise<ApiAuthResult> {
  try {
    const payload = await apiFetch<{ status?: string; user?: AuthUser; code?: string; message?: string }>('/api/auth/telegram', {
      method: 'POST',
      body: JSON.stringify({ initData, inviteToken }),
    })

    if (payload.user) {
      return { ok: true, user: payload.user }
    }

    return {
      ok: false,
      code: payload.code ?? 'unknown_error',
      message: payload.message ?? 'authentication failed',
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : 'authentication failed'
    return {
      ok: false,
      code: 'request_error',
      message,
    }
  }
}

export async function logoutFromApp(): Promise<boolean> {
  try {
    await apiFetch<{ status?: string }>('/api/auth/logout', { method: 'POST' })
    return true
  } catch {
    return false
  }
}

export async function logoutAllSessions(): Promise<boolean> {
  try {
    await apiFetch<{ status?: string }>('/api/auth/logout-all', { method: 'POST' })
    return true
  } catch {
    return false
  }
}

export async function getInvitations(): Promise<InvitationList> {
  return apiFetch<InvitationList>('/api/invitations', { method: 'GET' })
}

export async function createInvitation(): Promise<{ status: string; invitation: InvitationItem }> {
  return apiFetch<{ status: string; invitation: InvitationItem }>('/api/invitations', { method: 'POST' })
}

export async function revokeInvitation(invitationId: string): Promise<boolean> {
  try {
    await apiFetch<{ status?: string }>(`/api/invitations/${invitationId}`, { method: 'DELETE' })
    return true
  } catch {
    return false
  }
}

export async function getAdminUsers(): Promise<{ status: string; users: AuthUser[] }> {
  return apiFetch<{ status: string; users: AuthUser[] }>('/api/admin/users', { method: 'GET' })
}

export async function searchFriendsUsers(query: string): Promise<FriendProfile[]> {
  const search = new URLSearchParams({ q: query })
  const payload = await apiFetch<{ status: string; users: FriendProfile[] }>(`/api/users/search?${search.toString()}`, { method: 'GET' })
  return payload.users ?? []
}

export async function getFriendProfile(publicId: string): Promise<FriendProfile> {
  const payload = await apiFetch<{ status: string; user: FriendProfile }>(`/api/users/${encodeURIComponent(publicId)}`, { method: 'GET' })
  return payload.user
}

export async function getFriends(): Promise<FriendProfile[]> {
  const payload = await apiFetch<{ status: string; friends: FriendProfile[] }>('/api/friends', { method: 'GET' })
  return payload.friends ?? []
}

export async function getFriendRequests(direction: 'incoming' | 'outgoing'): Promise<FriendRequestItem[]> {
  const payload = await apiFetch<{ status: string; requests: FriendRequestItem[] }>(`/api/friends/requests/${direction}`, { method: 'GET' })
  return payload.requests ?? []
}

export async function createFriendRequest(targetPublicId: string): Promise<FriendRequestItem> {
  const payload = await apiFetch<{ status: string; request: FriendRequestItem }>('/api/friends', {
    method: 'POST',
    body: JSON.stringify({ target_public_id: targetPublicId }),
  })
  return payload.request
}

export async function respondToFriendRequest(requestId: string, action: 'accept' | 'reject' | 'cancel'): Promise<void> {
  await apiFetch<{ status: string }>(`/api/friends/requests/${encodeURIComponent(requestId)}/${action}`, { method: 'POST' })
}

export async function removeFriend(publicId: string): Promise<void> {
  await apiFetch<{ status: string }>(`/api/friends/${encodeURIComponent(publicId)}`, { method: 'DELETE' })
}

export async function getFriendPrivacySettings(): Promise<FriendPrivacySettings> {
  const payload = await apiFetch<{ status: string; settings: FriendPrivacySettings }>('/api/me/privacy', { method: 'GET' })
  return payload.settings
}

export async function updateFriendPrivacySettings(settings: FriendPrivacySettings): Promise<FriendPrivacySettings> {
  const payload = await apiFetch<{ status: string; settings: FriendPrivacySettings }>('/api/me/privacy', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
  return payload.settings
}

export async function getAvailability(from: string, to: string): Promise<{ timezone: string; days: AvailabilityDay[] }> {
  return apiFetch(`/api/availability?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`)
}

export async function setAvailabilityDay(date: string, state: AvailabilityDay['state']): Promise<void> {
  await apiFetch(`/api/availability/days/${encodeURIComponent(date)}`, { method: 'PUT', body: JSON.stringify({ state }) })
}

export async function addAvailabilityInterval(date: string, interval: Omit<AvailabilityInterval, 'id'>): Promise<AvailabilityInterval> {
  const payload = await apiFetch<{ interval: AvailabilityInterval }>(`/api/availability/days/${encodeURIComponent(date)}/intervals`, {
    method: 'POST', body: JSON.stringify(interval),
  })
  return payload.interval
}

export async function deleteAvailabilityInterval(id: string): Promise<void> {
  await apiFetch(`/api/availability/intervals/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function updateAvailabilityInterval(id: string, interval: Omit<AvailabilityInterval, 'id'>): Promise<AvailabilityInterval> {
  const payload = await apiFetch<{ interval: AvailabilityInterval }>(`/api/availability/intervals/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(interval) })
  return payload.interval
}

export async function getAvailabilityRules(): Promise<AvailabilityRule[]> {
  const payload = await apiFetch<{ rules: AvailabilityRule[] }>('/api/availability/rules')
  return payload.rules
}

export async function addAvailabilityRule(rule: Omit<AvailabilityRule, 'id' | 'is_active'>): Promise<AvailabilityRule> {
  const payload = await apiFetch<{ rule: AvailabilityRule }>('/api/availability/rules', { method: 'POST', body: JSON.stringify(rule) })
  return payload.rule
}

export async function deleteAvailabilityRule(id: string): Promise<void> {
  await apiFetch(`/api/availability/rules/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function getFriendAvailability(date: string, publicIds: string[]): Promise<FriendAvailability[]> {
  const query = new URLSearchParams({ date, user_ids: publicIds.join(',') })
  const payload = await apiFetch<{ friends: FriendAvailability[] }>(`/api/availability/friends?${query}`)
  return payload.friends
}

export async function getAvailabilityPrivacySettings(): Promise<{ visibility: 'friends_exact' | 'selected_exact' | 'busy_only'; timezone: string; exact_for: string[] }> {
  const payload = await apiFetch<{ settings: { visibility: 'friends_exact' | 'selected_exact' | 'busy_only'; timezone: string; exact_for: string[] } }>('/api/me/availability-privacy')
  return payload.settings
}

export async function updateAvailabilityPrivacySettings(settings: { visibility: 'friends_exact' | 'selected_exact' | 'busy_only'; timezone: string; exact_for: string[] }): Promise<void> {
  await apiFetch('/api/me/availability-privacy', { method: 'PUT', body: JSON.stringify(settings) })
}

export async function getWalks(status?: string): Promise<WalkEvent[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : ''
  const payload = await apiFetch<{ walks: WalkEvent[] }>(`/api/walks${query}`)
  return payload.walks
}

export async function createWalk(input: { title: string; description: string; scheduled_start: string; scheduled_end: string; timezone: string; location_name: string; latitude?: number; longitude?: number; participants: string[] }): Promise<WalkEvent> {
  const payload = await apiFetch<{ walk: WalkEvent }>('/api/walks', { method: 'POST', body: JSON.stringify(input) })
  return payload.walk
}

export async function findWalkSlots(date: string, duration_minutes: number, participants: string[]): Promise<WalkTimeSlot[]> {
  const payload = await apiFetch<{ slots: WalkTimeSlot[] }>('/api/walks/slots', { method: 'POST', body: JSON.stringify({ date, duration_minutes, participants }) })
  return payload.slots
}

export async function getWalk(id: string): Promise<WalkEvent> {
  const payload = await apiFetch<{ walk: WalkEvent }>(`/api/walks/${encodeURIComponent(id)}`)
  return payload.walk
}

export async function respondToWalk(id: string, action: 'accept' | 'reject'): Promise<void> {
  await apiFetch(`/api/walks/${encodeURIComponent(id)}/respond`, { method: 'POST', body: JSON.stringify({ action }) })
}

export async function cancelWalk(id: string): Promise<void> {
  await apiFetch(`/api/walks/${encodeURIComponent(id)}/cancel`, { method: 'POST' })
}

export async function rescheduleWalk(id: string, start: string, end: string, location: string, latitude?: number, longitude?: number): Promise<void> {
  await apiFetch(`/api/walks/${encodeURIComponent(id)}/reschedule`, { method: 'POST', body: JSON.stringify({ scheduled_start: start, scheduled_end: end, location_name: location, latitude, longitude }) })
}

export async function completeWalk(id: string): Promise<void> {
  await apiFetch(`/api/walks/${encodeURIComponent(id)}/complete`, { method: 'POST' })
}

export async function getWalkCalendar(id: string): Promise<{ google: string; outlook: string }> {
  const payload = await apiFetch<{ links: { google: string; outlook: string } }>(`/api/walks/${encodeURIComponent(id)}/calendar`)
  return payload.links
}

export function downloadWalkICS(id: string): void {
  const base = API_BASE_URL || window.location.origin
  window.location.assign(`${base}/api/walks/${encodeURIComponent(id)}/calendar?format=ics`)
}

export async function updateDisplayName(display_name: string): Promise<void> {
  await apiFetch('/api/me/profile', { method: 'PUT', body: JSON.stringify({ display_name }) })
}

export async function getNotificationSettings(): Promise<NotificationSettings> {
  const payload = await apiFetch<{ settings: NotificationSettings }>('/api/me/notifications')
  return payload.settings
}

export async function updateNotificationSettings(settings: NotificationSettings): Promise<void> {
  await apiFetch('/api/me/notifications', { method: 'PUT', body: JSON.stringify(settings) })
}

export async function searchPlaces(query: string): Promise<PlaceSuggestion[]> {
  const params = new URLSearchParams({ q: query })
  const payload = await apiFetch<{ places: PlaceSuggestion[] }>(`/api/places/search?${params}`)
  return payload.places
}

export async function getFavoritePlaces(): Promise<FavoritePlace[]> {
  const payload = await apiFetch<{ places: FavoritePlace[] }>('/api/places')
  return payload.places
}

export async function saveFavoritePlace(place: PlaceSuggestion): Promise<FavoritePlace> {
  const payload = await apiFetch<{ place: FavoritePlace }>('/api/places', { method: 'POST', body: JSON.stringify(place) })
  return payload.place
}

export async function deleteFavoritePlace(id: string): Promise<void> {
  await apiFetch(`/api/places/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
