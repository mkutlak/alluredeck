import { describe, it, expect, beforeEach } from 'vitest'
import { useAuthStore, selectIsAdmin, selectIsEditor, type Role } from '../auth'

const getState = () => useAuthStore.getState()

describe('useAuthStore', () => {
  beforeEach(() => {
    getState().clearAuth()
  })

  it.each([
    [undefined, 'local'],
    ['oidc', 'oidc'],
  ] as const)(
    'setAuth with provider %s stores a %s session expiring in expiresIn',
    (provider, expected) => {
      const before = Date.now()
      getState().setAuth(['editor'], 'sso-user', 3600, provider)

      expect(getState()).toMatchObject({
        isAuthenticated: true,
        roles: ['editor'],
        username: 'sso-user',
        provider: expected,
      })
      expect(getState().expiresAt).toBeGreaterThanOrEqual(before + 3_600_000)
    },
  )

  it('clearAuth resets all auth state including provider', () => {
    getState().setAuth(['admin'], 'admin-user', 3600, 'oidc')
    getState().clearAuth()

    expect(getState()).toMatchObject({
      isAuthenticated: false,
      roles: [],
      username: null,
      provider: null,
      expiresAt: null,
    })
  })

  it.each<[Role[], boolean, boolean]>([
    [['admin'], true, true],
    [['editor'], false, true],
    [['viewer'], false, false],
    [[], false, false],
  ])('roles %j -> isAdmin %s, isEditor %s', (roles, isAdmin, isEditor) => {
    useAuthStore.setState({ roles })
    expect(selectIsAdmin(getState())).toBe(isAdmin)
    expect(selectIsEditor(getState())).toBe(isEditor)
  })
})
