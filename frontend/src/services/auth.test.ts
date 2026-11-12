import { afterEach, describe, expect, it, vi } from 'vitest'
import { loginWithTelegram } from './auth'

describe('Telegram authentication request', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends initData and the invite token to the existing auth endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ user: { id: 'user-1' } }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await loginWithTelegram('signed-init-data', 'telegram-startapp-token')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, options] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/auth/telegram')
    expect(options.method).toBe('POST')
    expect(JSON.parse(String(options.body))).toEqual({
      initData: 'signed-init-data',
      inviteToken: 'telegram-startapp-token',
    })
  })
})