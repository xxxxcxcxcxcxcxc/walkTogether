import { useState, useEffect, type ReactNode } from 'react'
import { Home, Calendar, MapPin, Users, User, ShieldAlert, LoaderCircle } from 'lucide-react'
import { HomePage } from './pages/home/HomePage'
import { AvailabilityPage } from './pages/availability/AvailabilityPage'
import { WalksPage } from './pages/walks/WalksPage'
import { FriendsPage } from './pages/friends/FriendsPage'
import { ProfilePage } from './pages/profile/ProfilePage'
import { BottomNavigation } from './components/BottomNavigation'
import { useTelegramTheme } from './hooks/useTelegramTheme'
import { getInitData, getStartParam, initTelegramApp, isTelegramEnvironment } from './services/telegram'
import { getCurrentUser, loginWithTelegram, type AuthUser } from './services/auth'

type Tab = 'home' | 'availability' | 'walks' | 'friends' | 'profile'

const tabs: { id: Tab; label: string; icon: ReactNode }[] = [
  { id: 'home', label: 'Главная', icon: <Home size={24} strokeWidth={2} /> },
  { id: 'availability', label: 'Расписание', icon: <Calendar size={24} strokeWidth={2} /> },
  { id: 'walks', label: 'Прогулки', icon: <MapPin size={24} strokeWidth={2} /> },
  { id: 'friends', label: 'Друзья', icon: <Users size={24} strokeWidth={2} /> },
  { id: 'profile', label: 'Профиль', icon: <User size={24} strokeWidth={2} /> },
]

type AuthState =
  | { status: 'loading' }
  | { status: 'outside-telegram' }
  | { status: 'auth-required' }
  | { status: 'invitation-required'; message: string }
  | { status: 'error'; message: string }
  | { status: 'authenticated'; user: AuthUser }

export function App() {
  const [activeTab, setActiveTab] = useState<Tab>('home')
  const [authState, setAuthState] = useState<AuthState>({ status: 'loading' })
  const { bgColor, isDark } = useTelegramTheme()

  useEffect(() => {
    initTelegramApp()

    const bootstrap = async () => {
      try {
        const currentUser = await getCurrentUser()
        if (currentUser) {
          setAuthState({ status: 'authenticated', user: currentUser })
          return
        }

        if (!isTelegramEnvironment()) {
          setAuthState({ status: 'outside-telegram' })
          return
        }

        const initData = getInitData()
        if (!initData) {
          setAuthState({ status: 'auth-required' })
          return
        }

        const inviteToken = getStartParam()
        const result = await loginWithTelegram(initData, inviteToken || undefined)
        if (result.ok && result.user) {
          setAuthState({ status: 'authenticated', user: result.user })
          return
        }

        if (result.code === 'invitation_required') {
          setAuthState({ status: 'invitation-required', message: result.message ?? 'Для входа требуется действующее приглашение.' })
          return
        }

        setAuthState({ status: 'error', message: result.message ?? 'Не удалось выполнить вход.' })
      } catch {
        setAuthState({ status: 'error', message: 'Не удалось установить безопасное соединение с сервером.' })
      }
    }

    void bootstrap()
  }, [])

  const renderPage = (user: AuthUser) => {
    switch (activeTab) {
      case 'home':
        return <HomePage user={user} onOpenWalks={() => setActiveTab('walks')} onOpenSchedule={() => setActiveTab('availability')} />
      case 'availability':
        return <AvailabilityPage />
      case 'walks':
        return <WalksPage />
      case 'friends':
        return <FriendsPage />
      case 'profile':
        return <ProfilePage user={user} onLogout={() => setAuthState({ status: 'auth-required' })} />
    }
  }

  if (authState.status === 'loading') {
    return (
      <div className="min-h-screen flex items-center justify-center px-6" style={{ backgroundColor: bgColor }}>
        <div className="flex flex-col items-center gap-3 text-center text-slate-700">
          <LoaderCircle className="animate-spin" size={28} />
          <p className="text-base font-medium">Проверка авторизации…</p>
        </div>
      </div>
    )
  }

  if (authState.status === 'outside-telegram') {
    return (
      <div className="min-h-screen flex items-center justify-center px-6" style={{ backgroundColor: bgColor }}>
        <div className="max-w-sm rounded-2xl bg-white/80 p-6 text-center shadow-sm backdrop-blur-sm">
          <ShieldAlert className="mx-auto mb-4 text-amber-500" size={32} />
          <h2 className="text-xl font-semibold text-slate-900">Открыть через Telegram</h2>
          <p className="mt-2 text-sm leading-6 text-slate-600">
            Это приложение работает только внутри Telegram Mini App. Откройте его из Telegram, чтобы продолжить.
          </p>
        </div>
      </div>
    )
  }

  if (authState.status === 'auth-required') {
    return (
      <div className="min-h-screen flex items-center justify-center px-6" style={{ backgroundColor: bgColor }}>
        <div className="max-w-sm rounded-2xl bg-white/80 p-6 text-center shadow-sm backdrop-blur-sm">
          <ShieldAlert className="mx-auto mb-4 text-blue-500" size={32} />
          <h2 className="text-xl font-semibold text-slate-900">Требуется Telegram initData</h2>
          <p className="mt-2 text-sm leading-6 text-slate-600">
            В этом окружении не удалось получить корректные данные авторизации Telegram. Проверьте запуск внутри Mini App.
          </p>
        </div>
      </div>
    )
  }

  if (authState.status === 'invitation-required') {
    return (
      <div className="min-h-screen flex items-center justify-center px-6" style={{ backgroundColor: bgColor }}>
        <div className="max-w-sm rounded-2xl bg-white/80 p-6 text-center shadow-sm backdrop-blur-sm">
          <ShieldAlert className="mx-auto mb-4 text-violet-500" size={32} />
          <h2 className="text-xl font-semibold text-slate-900">Доступ закрыт</h2>
          <p className="mt-2 text-sm leading-6 text-slate-600">{authState.message}</p>
        </div>
      </div>
    )
  }

  if (authState.status === 'error') {
    return (
      <div className="min-h-screen flex items-center justify-center px-6" style={{ backgroundColor: bgColor }}>
        <div className="max-w-sm rounded-2xl bg-white/80 p-6 text-center shadow-sm backdrop-blur-sm">
          <ShieldAlert className="mx-auto mb-4 text-red-500" size={32} />
          <h2 className="text-xl font-semibold text-slate-900">Ошибка авторизации</h2>
          <p className="mt-2 text-sm leading-6 text-slate-600">{authState.message}</p>
        </div>
      </div>
    )
  }

  return (
    <div className={`min-h-screen flex flex-col ${isDark ? 'dark' : ''}`} style={{ backgroundColor: bgColor }}>
      <div className="flex-1 overflow-y-auto pb-16">
        {renderPage(authState.user)}
      </div>
      <BottomNavigation tabs={tabs} activeTab={activeTab} onTabChange={setActiveTab} />
    </div>
  )
}
