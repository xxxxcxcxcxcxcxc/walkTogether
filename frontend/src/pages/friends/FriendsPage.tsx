import { useEffect, useState } from 'react'
import { Check, Copy, LoaderCircle, RefreshCw, Search, Send, Shield, UserPlus, UserRoundMinus, Users, X } from 'lucide-react'
import { useTelegramTheme } from '../../hooks/useTelegramTheme'
import {
  createFriendRequest,
  createInvitation,
  getFriendPrivacySettings,
  getFriendProfile,
  getFriendRequests,
  getFriends,
  getInvitations,
  removeFriend,
  respondToFriendRequest,
  revokeInvitation,
  searchFriendsUsers,
  updateFriendPrivacySettings,
  type FriendPrivacySettings,
  type FriendProfile,
  type FriendRequestItem,
  type InvitationItem,
} from '../../services/auth'

type Section = 'friends' | 'requests' | 'invitations'
type Direction = 'incoming' | 'outgoing'

const emptyPrivacy: FriendPrivacySettings = { visibility: 'all', friend_notifications_enabled: true }

export function FriendsPage() {
  const { bgColor, textColor, hintColor, secondaryBgColor } = useTelegramTheme()
  const [section, setSection] = useState<Section>('friends')
  const [direction, setDirection] = useState<Direction>('incoming')
  const [friends, setFriends] = useState<FriendProfile[]>([])
  const [friendsLoading, setFriendsLoading] = useState(true)
  const [friendsError, setFriendsError] = useState<string | null>(null)
  const [incoming, setIncoming] = useState<FriendRequestItem[]>([])
  const [outgoing, setOutgoing] = useState<FriendRequestItem[]>([])
  const [requestsLoading, setRequestsLoading] = useState(true)
  const [requestsError, setRequestsError] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [searchRevision, setSearchRevision] = useState(0)
  const [searchResults, setSearchResults] = useState<FriendProfile[]>([])
  const [searchLoading, setSearchLoading] = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)
  const [privacy, setPrivacy] = useState<FriendPrivacySettings>(emptyPrivacy)
  const [privacyLoading, setPrivacyLoading] = useState(true)
  const [privacyError, setPrivacyError] = useState<string | null>(null)
  const [invitations, setInvitations] = useState<InvitationItem[]>([])
  const [slotsUsed, setSlotsUsed] = useState(0)
  const [invitationsLoading, setInvitationsLoading] = useState(true)
  const [invitationsError, setInvitationsError] = useState<string | null>(null)
  const [creatingInvite, setCreatingInvite] = useState(false)
  const [busyAction, setBusyAction] = useState('')
  const [actionError, setActionError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)
  const [profile, setProfile] = useState<FriendProfile | null>(null)
  const [profileLoading, setProfileLoading] = useState(false)
  const [profileError, setProfileError] = useState<string | null>(null)

  const refreshFriends = async () => {
    setFriendsLoading(true)
    setFriendsError(null)
    try {
      setFriends(await getFriends())
    } catch (error) {
      setFriendsError(displayError(error))
    } finally {
      setFriendsLoading(false)
    }
  }

  const refreshRequests = async () => {
    setRequestsLoading(true)
    setRequestsError(null)
    try {
      const [nextIncoming, nextOutgoing] = await Promise.all([
        getFriendRequests('incoming'),
        getFriendRequests('outgoing'),
      ])
      setIncoming(nextIncoming)
      setOutgoing(nextOutgoing)
    } catch (error) {
      setRequestsError(displayError(error))
    } finally {
      setRequestsLoading(false)
    }
  }

  const refreshInvitations = async () => {
    setInvitationsLoading(true)
    setInvitationsError(null)
    try {
      const payload = await getInvitations()
      setInvitations(payload.items ?? [])
      setSlotsUsed(payload.slots_used ?? 0)
    } catch (error) {
      setInvitationsError(displayError(error))
    } finally {
      setInvitationsLoading(false)
    }
  }

  const refreshPrivacy = async () => {
    setPrivacyLoading(true)
    setPrivacyError(null)
    try {
      setPrivacy(await getFriendPrivacySettings())
    } catch (error) {
      setPrivacyError(displayError(error))
    } finally {
      setPrivacyLoading(false)
    }
  }

  useEffect(() => {
    void refreshFriends()
    void refreshRequests()
    void refreshInvitations()
    void refreshPrivacy()
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(searchQuery.trim()), 300)
    return () => window.clearTimeout(timer)
  }, [searchQuery])

  useEffect(() => {
    let active = true
    if (debouncedQuery.length < 2) {
      setSearchResults([])
      setSearchError(null)
      setSearchLoading(false)
      return () => { active = false }
    }

    setSearchLoading(true)
    setSearchError(null)
    void searchFriendsUsers(debouncedQuery)
      .then((results) => { if (active) setSearchResults(results) })
      .catch((error: unknown) => { if (active) setSearchError(displayError(error)) })
      .finally(() => { if (active) setSearchLoading(false) })

    return () => { active = false }
  }, [debouncedQuery, searchRevision])

  const refreshAfterFriendAction = async () => {
    await Promise.all([refreshFriends(), refreshRequests()])
    setSearchRevision((value) => value + 1)
  }

  const runFriendAction = async (key: string, action: () => Promise<unknown>, message: string) => {
    setBusyAction(key)
    setActionError(null)
    setSuccessMessage(null)
    try {
      await action()
      setSuccessMessage(message)
      await refreshAfterFriendAction()
      if (profile) {
        try {
          setProfile(await getFriendProfile(profile.public_id))
        } catch {
          setProfile(null)
        }
      }
    } catch (error) {
      setActionError(displayError(error))
    } finally {
      setBusyAction('')
    }
  }

  const openProfile = async (candidate: FriendProfile) => {
    setProfile(candidate)
    setProfileError(null)
    setProfileLoading(true)
    try {
      setProfile(await getFriendProfile(candidate.public_id))
    } catch (error) {
      setProfileError(displayError(error))
    } finally {
      setProfileLoading(false)
    }
  }

  const handleCreateInvite = async () => {
    if (slotsUsed >= 5) return
    setCreatingInvite(true)
    setActionError(null)
    setSuccessMessage(null)
    try {
      const payload = await createInvitation()
      setInvitations((current) => [payload.invitation, ...current])
      setSlotsUsed((current) => current + 1)
      setSuccessMessage('Приглашение создано.')
    } catch (error) {
      setActionError(displayError(error))
    } finally {
      setCreatingInvite(false)
    }
  }

  const handleRevokeInvite = async (item: InvitationItem) => {
    setBusyAction(item.id)
    setActionError(null)
    try {
      const revoked = await revokeInvitation(item.id)
      if (!revoked) throw new Error('Не удалось отозвать приглашение.')
      setInvitations((current) => current.map((invitation) => invitation.id === item.id ? { ...invitation, status: 'revoked' } : invitation))
      setSlotsUsed((current) => Math.max(0, current - 1))
      setSuccessMessage('Приглашение отозвано.')
    } catch (error) {
      setActionError(displayError(error))
    } finally {
      setBusyAction('')
    }
  }

  const handleCopyLink = async (item: InvitationItem) => {
    if (!item.deep_link) return
    try {
      await navigator.clipboard.writeText(item.deep_link)
      setSuccessMessage('Ссылка скопирована.')
      setActionError(null)
    } catch {
      setActionError('Не удалось скопировать ссылку.')
    }
  }

  const savePrivacy = async (next: FriendPrivacySettings) => {
    setPrivacyLoading(true)
    setPrivacyError(null)
    try {
      setPrivacy(await updateFriendPrivacySettings(next))
      setSuccessMessage('Настройки приватности сохранены.')
    } catch (error) {
      setPrivacyError(displayError(error))
    } finally {
      setPrivacyLoading(false)
    }
  }

  const currentRequests = direction === 'incoming' ? incoming : outgoing

  return (
    <div className="mx-auto max-w-2xl space-y-5 p-4" style={{ backgroundColor: bgColor, color: textColor }}>
      <header>
        <h1 className="text-2xl font-bold">Друзья</h1>
      </header>

      <nav className="flex gap-2 overflow-x-auto border-b pb-3" aria-label="Разделы друзей">
        <SectionButton active={section === 'friends'} onClick={() => setSection('friends')}>Друзья ({friends.length})</SectionButton>
        <SectionButton active={section === 'requests'} onClick={() => setSection('requests')}>Запросы ({incoming.length})</SectionButton>
        <SectionButton active={section === 'invitations'} onClick={() => setSection('invitations')}>Приглашения ({slotsUsed}/5)</SectionButton>
      </nav>

      {actionError && <p className="rounded-lg border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800" role="alert">{actionError}</p>}
      {successMessage && <p className="rounded-lg border border-green-300 bg-green-50 px-3 py-2 text-sm text-green-800" role="status">{successMessage}</p>}

      {section === 'friends' && (
        <section className="space-y-4" aria-label="Мои друзья и поиск">
          <div>
            <label htmlFor="friend-search" className="mb-2 block text-sm font-medium">Найти участника</label>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2" size={18} style={{ color: hintColor }} />
              <input
                id="friend-search"
                type="search"
                value={searchQuery}
                onChange={(event) => setSearchQuery(event.target.value)}
                placeholder="Имя или username"
                className="input pl-10"
                autoComplete="off"
              />
            </div>
          </div>

          {searchQuery.trim().length > 0 ? (
            <div className="space-y-3">
              {searchQuery.trim().length < 2 && <p className="text-sm" style={{ color: hintColor }}>Введите минимум 2 символа.</p>}
              {debouncedQuery.length >= 2 && searchLoading && <LoadingState label="Поиск…" />}
              {searchError && <RetryState message={searchError} onRetry={() => setSearchRevision((value) => value + 1)} />}
              {debouncedQuery.length >= 2 && !searchLoading && !searchError && searchResults.length === 0 && <EmptyState icon={<Search size={28} />} title="Ничего не найдено" detail="Попробуйте изменить запрос." />}
              {!searchLoading && !searchError && searchResults.map((item) => (
                <ProfileRow key={item.public_id} profile={item} hintColor={hintColor} secondaryBgColor={secondaryBgColor} onOpen={() => void openProfile(item)}>
                  <RelationshipAction
                    profile={item}
                    busy={busyAction === item.public_id}
                    onAdd={() => void runFriendAction(item.public_id, () => createFriendRequest(item.public_id), 'Запрос отправлен.')}
                    onIncoming={() => { setDirection('incoming'); setSection('requests') }}
                  />
                </ProfileRow>
              ))}
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <h2 className="text-lg font-semibold">Мои друзья</h2>
                <button className="btn-ghost p-2" title="Обновить список друзей" aria-label="Обновить список друзей" onClick={() => void refreshFriends()}>
                  <RefreshCw size={17} />
                </button>
              </div>
              {friendsLoading && <LoadingState label="Загрузка друзей…" />}
              {friendsError && <RetryState message={friendsError} onRetry={() => void refreshFriends()} />}
              {!friendsLoading && !friendsError && friends.length === 0 && <EmptyState icon={<Users size={30} />} title="Пока нет друзей" detail="Найдите знакомых среди участников приложения." />}
              {!friendsLoading && !friendsError && friends.map((friend) => (
                <ProfileRow key={friend.public_id} profile={friend} hintColor={hintColor} secondaryBgColor={secondaryBgColor} onOpen={() => void openProfile(friend)}>
                  <button
                    className="btn-ghost p-2 text-red-600"
                    title="Удалить из друзей"
                    aria-label={`Удалить ${friend.display_name} из друзей`}
                    disabled={busyAction === friend.public_id}
                    onClick={() => void runFriendAction(friend.public_id, () => removeFriend(friend.public_id), 'Пользователь удалён из друзей.')}
                  >
                    <UserRoundMinus size={18} />
                  </button>
                </ProfileRow>
              ))}
            </div>
          )}

          <PrivacySettings
            settings={privacy}
            loading={privacyLoading}
            error={privacyError}
            hintColor={hintColor}
            secondaryBgColor={secondaryBgColor}
            onChange={(next) => void savePrivacy(next)}
            onRetry={() => void refreshPrivacy()}
          />
        </section>
      )}

      {section === 'requests' && (
        <section className="space-y-4" aria-label="Запросы в друзья">
          <div className="flex gap-2" role="group" aria-label="Направление запросов">
            <SectionButton active={direction === 'incoming'} onClick={() => setDirection('incoming')}>Входящие ({incoming.length})</SectionButton>
            <SectionButton active={direction === 'outgoing'} onClick={() => setDirection('outgoing')}>Исходящие ({outgoing.length})</SectionButton>
          </div>
          {requestsLoading && <LoadingState label="Загрузка запросов…" />}
          {requestsError && <RetryState message={requestsError} onRetry={() => void refreshRequests()} />}
          {!requestsLoading && !requestsError && currentRequests.length === 0 && (
            <EmptyState icon={<UserPlus size={30} />} title={direction === 'incoming' ? 'Входящих запросов нет' : 'Исходящих запросов нет'} detail="Новые запросы появятся здесь." />
          )}
          {!requestsLoading && !requestsError && currentRequests.map((request) => (
            <ProfileRow key={request.id} profile={request.user} hintColor={hintColor} secondaryBgColor={secondaryBgColor} onOpen={() => void openProfile(request.user)}>
              {direction === 'incoming' ? (
                <div className="flex gap-1">
                  <button className="btn-ghost p-2 text-green-700" title="Принять запрос" aria-label={`Принять запрос от ${request.user.display_name}`} disabled={busyAction === request.id} onClick={() => void runFriendAction(request.id, () => respondToFriendRequest(request.id, 'accept'), 'Запрос принят.')}><Check size={18} /></button>
                  <button className="btn-ghost p-2 text-red-600" title="Отклонить запрос" aria-label={`Отклонить запрос от ${request.user.display_name}`} disabled={busyAction === request.id} onClick={() => void runFriendAction(request.id, () => respondToFriendRequest(request.id, 'reject'), 'Запрос отклонён.')}><X size={18} /></button>
                </div>
              ) : (
                <button className="btn-ghost p-2" title="Отменить запрос" aria-label={`Отменить запрос для ${request.user.display_name}`} disabled={busyAction === request.id} onClick={() => void runFriendAction(request.id, () => respondToFriendRequest(request.id, 'cancel'), 'Запрос отменён.')}><X size={18} /></button>
              )}
            </ProfileRow>
          ))}
        </section>
      )}

      {section === 'invitations' && (
        <section className="space-y-4" aria-label="Приглашения в приложение">
          <div className="rounded-lg border p-4" style={{ backgroundColor: secondaryBgColor }}>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="font-semibold">Слоты приглашений</h2>
              <span className="badge badge-primary">{slotsUsed}/5</span>
            </div>
            <div className="mb-3 flex gap-2" aria-hidden="true">
              {Array.from({ length: 5 }).map((_, index) => <span key={index} className={`h-2 flex-1 rounded-full ${index < slotsUsed ? 'bg-primary-500' : 'bg-gray-200 dark:bg-gray-700'}`} />)}
            </div>
            <button className="btn-primary w-full text-sm" onClick={() => void handleCreateInvite()} disabled={creatingInvite || invitationsLoading || slotsUsed >= 5}>
              <UserPlus size={16} />
              <span>{creatingInvite ? 'Создаём…' : 'Создать приглашение'}</span>
            </button>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold">История приглашений</h2>
              <button className="btn-ghost p-2" title="Обновить приглашения" aria-label="Обновить приглашения" onClick={() => void refreshInvitations()}><RefreshCw size={17} /></button>
            </div>
            {invitationsLoading && <LoadingState label="Загрузка приглашений…" />}
            {invitationsError && <RetryState message={invitationsError} onRetry={() => void refreshInvitations()} />}
            {!invitationsLoading && !invitationsError && invitations.length === 0 && <EmptyState icon={<Send size={30} />} title="Приглашений пока нет" detail="Создайте приглашение, чтобы позвать знакомого." />}
            {!invitationsLoading && !invitationsError && invitations.map((item) => (
              <div key={item.id} className="flex items-center gap-3 rounded-lg border p-3" style={{ backgroundColor: secondaryBgColor }}>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="font-medium">{invitationStatus(item.status)}</h3>
                    <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">{item.status}</span>
                  </div>
                  <p className="mt-1 text-xs" style={{ color: hintColor }}>
                    {item.created_at ? new Date(item.created_at).toLocaleDateString('ru-RU') : '—'} · {item.expires_at ? `до ${new Date(item.expires_at).toLocaleDateString('ru-RU')}` : 'без ограничения'}
                  </p>
                </div>
                {item.deep_link && <button className="btn-ghost p-2" title="Копировать ссылку" aria-label="Копировать ссылку приглашения" onClick={() => void handleCopyLink(item)}><Copy size={18} /></button>}
                {item.status === 'pending' && <button className="btn-ghost p-2 text-red-600" title="Отозвать приглашение" aria-label="Отозвать приглашение" disabled={busyAction === item.id} onClick={() => void handleRevokeInvite(item)}><X size={18} /></button>}
              </div>
            ))}
          </div>
        </section>
      )}

      {profile && (
        <ProfileDialog
          profile={profile}
          loading={profileLoading}
          error={profileError}
          busyAction={busyAction}
          onClose={() => { setProfile(null); setProfileError(null) }}
          onRetry={() => void openProfile(profile)}
          onAdd={() => void runFriendAction(profile.public_id, () => createFriendRequest(profile.public_id), 'Запрос отправлен.')}
          onRespond={(action) => profile.request_id && void runFriendAction(profile.request_id, () => respondToFriendRequest(profile.request_id!, action), action === 'accept' ? 'Запрос принят.' : action === 'reject' ? 'Запрос отклонён.' : 'Запрос отменён.')}
          onRemove={() => void runFriendAction(profile.public_id, () => removeFriend(profile.public_id), 'Пользователь удалён из друзей.')}
        />
      )}
    </div>
  )
}

function displayError(error: unknown): string {
  const message = error instanceof Error ? error.message : 'Не удалось выполнить запрос.'
  return /unauthorized|authentication required/i.test(message) ? 'Сессия истекла. Закройте и снова откройте Mini App.' : message
}

function invitationStatus(status: InvitationItem['status']): string {
  switch (status) {
    case 'pending': return 'Ожидает регистрации'
    case 'accepted': return 'Принято'
    case 'revoked': return 'Отозвано'
    case 'rejected': return 'Отклонено'
    case 'expired': return 'Истекло'
  }
}

function SectionButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" aria-pressed={active} onClick={onClick} className={`shrink-0 border-b-2 px-2 py-2 text-sm font-medium ${active ? 'border-primary-500 text-primary-700' : 'border-transparent'}`}>{children}</button>
}

function LoadingState({ label }: { label: string }) {
  return <p className="flex items-center justify-center gap-2 py-5 text-sm" role="status"><LoaderCircle size={17} className="animate-spin" />{label}</p>
}

function RetryState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="space-y-3 rounded-lg border border-red-300 bg-red-50 p-4 text-center text-sm text-red-800" role="alert"><p>{message}</p><button className="btn-ghost inline-flex items-center gap-2" onClick={onRetry}><RefreshCw size={16} />Повторить</button></div>
}

function EmptyState({ icon, title, detail }: { icon: React.ReactNode; title: string; detail: string }) {
  return <div className="py-10 text-center"><div className="mb-3 flex justify-center" aria-hidden="true">{icon}</div><h3 className="font-medium">{title}</h3><p className="mt-1 text-sm text-gray-500">{detail}</p></div>
}

function Avatar({ profile }: { profile: FriendProfile }) {
  return <div className="flex h-11 w-11 shrink-0 items-center justify-center overflow-hidden rounded-full bg-primary-100 font-semibold text-primary-700">{profile.avatar_url ? <img src={profile.avatar_url} alt="" className="h-full w-full object-cover" /> : profile.display_name.slice(0, 1).toUpperCase()}</div>
}

function ProfileRow({ profile, hintColor, secondaryBgColor, onOpen, children }: { profile: FriendProfile; hintColor: string; secondaryBgColor: string; onOpen: () => void; children: React.ReactNode }) {
  return <div className="flex items-center gap-3 rounded-lg border p-3" style={{ backgroundColor: secondaryBgColor }}>
    <Avatar profile={profile} />
    <button className="min-w-0 flex-1 text-left" onClick={onOpen}>
      <span className="block truncate font-medium">{profile.display_name}</span>
      {profile.username && <span className="block truncate text-sm" style={{ color: hintColor }}>@{profile.username.replace(/^@/, '')}</span>}
    </button>
    {children}
  </div>
}

function RelationshipAction({ profile, busy, onAdd, onIncoming }: { profile: FriendProfile; busy: boolean; onAdd: () => void; onIncoming: () => void }) {
  if (profile.relationship_status === 'outgoing_pending') return <span className="text-xs text-gray-500">Отправлен</span>
  if (profile.relationship_status === 'incoming_pending') return <button className="btn-secondary text-xs" onClick={onIncoming}>Ответить</button>
  if (profile.relationship_status === 'accepted') return <span className="text-xs text-green-700">Друг</span>
  return <button className="btn-secondary inline-flex items-center gap-1 text-xs" disabled={busy} onClick={onAdd}><UserPlus size={15} />Добавить</button>
}

function PrivacySettings({ settings, loading, error, hintColor, secondaryBgColor, onChange, onRetry }: {
  settings: FriendPrivacySettings
  loading: boolean
  error: string | null
  hintColor: string
  secondaryBgColor: string
  onChange: (settings: FriendPrivacySettings) => void
  onRetry: () => void
}) {
  return <section className="space-y-3 border-t pt-4" aria-label="Настройки приватности">
    <div className="flex items-center gap-2"><Shield size={18} /><h2 className="font-semibold">Приватность</h2></div>
    {error ? <RetryState message={error} onRetry={onRetry} /> : loading ? <LoadingState label="Загрузка настроек…" /> : (
      <div className="space-y-3 rounded-lg border p-3" style={{ backgroundColor: secondaryBgColor }}>
        <label className="block text-sm font-medium" htmlFor="profile-visibility">Кто может найти профиль</label>
        <select id="profile-visibility" className="input" value={settings.visibility} onChange={(event) => onChange({ ...settings, visibility: event.target.value as FriendPrivacySettings['visibility'] })}>
          <option value="all">Все участники</option>
          <option value="friends">Только друзья</option>
          <option value="hidden">Скрыт из поиска</option>
        </select>
        <label className="flex items-center gap-2 text-sm" style={{ color: hintColor }}>
          <input type="checkbox" checked={settings.friend_notifications_enabled} onChange={(event) => onChange({ ...settings, friend_notifications_enabled: event.target.checked })} />
          Получать уведомления о запросах в друзья
        </label>
      </div>
    )}
  </section>
}

function ProfileDialog({ profile, loading, error, busyAction, onClose, onRetry, onAdd, onRespond, onRemove }: {
  profile: FriendProfile
  loading: boolean
  error: string | null
  busyAction: string
  onClose: () => void
  onRetry: () => void
  onAdd: () => void
  onRespond: (action: 'accept' | 'reject' | 'cancel') => void
  onRemove: () => void
}) {
  const { secondaryBgColor, hintColor } = useTelegramTheme()
  const busy = busyAction === profile.public_id || busyAction === profile.request_id
  return <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 p-3 sm:items-center" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <section className="w-full max-w-md space-y-4 rounded-xl bg-white p-5 text-slate-900 shadow-xl" role="dialog" aria-modal="true" aria-labelledby="friend-profile-title" style={{ backgroundColor: secondaryBgColor }}>
      <div className="flex items-center justify-between"><h2 id="friend-profile-title" className="text-lg font-semibold">Профиль участника</h2><button className="btn-ghost p-2" aria-label="Закрыть профиль" onClick={onClose}><X size={19} /></button></div>
      {loading && <LoadingState label="Загрузка профиля…" />}
      {error && <RetryState message={error} onRetry={onRetry} />}
      {!loading && !error && <>
        <div className="flex items-center gap-3"><Avatar profile={profile} /><div className="min-w-0"><p className="truncate font-semibold">{profile.display_name}</p>{profile.username && <p className="truncate text-sm" style={{ color: hintColor }}>@{profile.username.replace(/^@/, '')}</p>}<p className="mt-1 text-xs" style={{ color: hintColor }}>В приложении с {new Date(profile.joined_at).toLocaleDateString('ru-RU')}</p></div></div>
        <p className="text-sm">{relationshipLabel(profile.relationship_status)}</p>
        <div className="flex flex-wrap gap-2">
          {profile.relationship_status === 'none' && <button className="btn-primary" disabled={busy} onClick={onAdd}><UserPlus size={16} />Добавить в друзья</button>}
          {profile.relationship_status === 'outgoing_pending' && <button className="btn-secondary" disabled={busy} onClick={() => onRespond('cancel')}>Отменить запрос</button>}
          {profile.relationship_status === 'incoming_pending' && <><button className="btn-primary" disabled={busy} onClick={() => onRespond('accept')}><Check size={16} />Принять</button><button className="btn-ghost text-red-600" disabled={busy} onClick={() => onRespond('reject')}>Отклонить</button></>}
          {profile.relationship_status === 'accepted' && <button className="btn-ghost text-red-600" disabled={busy} onClick={onRemove}><UserRoundMinus size={16} />Удалить из друзей</button>}
        </div>
      </>}
    </section>
  </div>
}

function relationshipLabel(status: FriendProfile['relationship_status']): string {
  switch (status) {
    case 'accepted': return 'Вы друзья'
    case 'incoming_pending': return 'Ожидает вашего ответа'
    case 'outgoing_pending': return 'Запрос отправлен'
    case 'rejected': return 'Запрос отклонён'
    case 'cancelled': return 'Связь завершена'
    case 'none': return 'Вы пока не друзья'
  }
}
