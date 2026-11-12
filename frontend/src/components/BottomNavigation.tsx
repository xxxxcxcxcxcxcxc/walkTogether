import { type ReactNode } from 'react'
import { useTelegramTheme } from '../hooks/useTelegramTheme'

type TabId = 'home' | 'availability' | 'walks' | 'friends' | 'profile'

interface Tab {
  id: TabId
  label: string
  icon: ReactNode
}

interface BottomNavigationProps {
  tabs: Tab[]
  activeTab: TabId
  onTabChange: (tabId: TabId) => void
}

export function BottomNavigation({ tabs, activeTab, onTabChange }: BottomNavigationProps) {
  const { bgColor, hintColor } = useTelegramTheme()

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-50 safe-area-inset-bottom"
      style={{ backgroundColor: bgColor }}
    >
      <div className="flex items-center justify-around h-16 border-t border-gray-200 dark:border-gray-700">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => onTabChange(tab.id)}
            className={`flex flex-col items-center gap-1 px-3 py-2 rounded-xl transition-all duration-200 tap-highlight-transparent ${
              activeTab === tab.id
                ? 'text-primary-500'
                : `text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200`
            }`}
            style={{
              color: activeTab === tab.id ? '#0ea5e9' : hintColor,
            }}
            aria-current={activeTab === tab.id ? 'page' : undefined}
            aria-label={tab.label}
          >
            <div className="relative" style={{ color: 'currentColor' }}>
              {tab.icon}
              {activeTab === tab.id && (
                <span className="absolute -bottom-1 left-1/2 -translate-x-1/2 w-1.5 h-1.5 rounded-full" style={{ backgroundColor: '#0ea5e9' }} />
              )}
            </div>
            <span className="text-xs font-medium leading-none">{tab.label}</span>
          </button>
        ))}
      </div>
    </nav>
  )
}