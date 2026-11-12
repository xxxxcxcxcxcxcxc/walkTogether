import { useState, useEffect } from 'react'
import { getThemeParams, getColorScheme, onThemeChange, offThemeChange } from '../services/telegram'

export function useTelegramTheme() {
  const [themeParams, setThemeParams] = useState(getThemeParams())
  const [colorScheme, setColorScheme] = useState<'light' | 'dark'>(getColorScheme())

  useEffect(() => {
    const handleThemeChange = (params: ReturnType<typeof getThemeParams>) => {
      setThemeParams(params)
    }

    const handleColorSchemeChange = () => {
      setColorScheme(getColorScheme())
      setThemeParams(getThemeParams())
    }

    onThemeChange(handleThemeChange)
    onThemeChange(handleColorSchemeChange)

    return () => {
      offThemeChange(handleThemeChange)
      offThemeChange(handleColorSchemeChange)
    }
  }, [])

  const isDark = colorScheme === 'dark'

  const bgColor = themeParams.bg_color || (isDark ? '#1a1a2e' : '#ffffff')
  const textColor = themeParams.text_color || (isDark ? '#ffffff' : '#1a1a2e')
  const hintColor = themeParams.hint_color || (isDark ? '#8b8b9e' : '#6b6b7b')
  const linkColor = themeParams.link_color || '#0ea5e9'
  const buttonColor = themeParams.button_color || '#0ea5e9'
  const buttonTextColor = themeParams.button_text_color || '#ffffff'
  const secondaryBgColor = themeParams.secondary_bg_color || (isDark ? '#25253a' : '#f5f5f5')

  return {
    themeParams,
    colorScheme,
    isDark,
    bgColor,
    textColor,
    hintColor,
    linkColor,
    buttonColor,
    buttonTextColor,
    secondaryBgColor,
  }
}