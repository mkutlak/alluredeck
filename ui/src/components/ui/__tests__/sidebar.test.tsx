import { readSidebarOpen } from '../sidebar'

function setCookie(value: string) {
  document.cookie = `${value}; path=/`
}

afterEach(() => {
  document.cookie = 'sidebar_state=; max-age=0; path=/'
  document.cookie = 'theme=; max-age=0; path=/'
})

describe('readSidebarOpen', () => {
  it('is open when the cookie was never written', () => {
    expect(readSidebarOpen()).toBe(true)
  })

  it.each([
    { cookie: 'sidebar_state=true', open: true },
    { cookie: 'sidebar_state=false', open: false },
    // A garbled value must not hide the navigation.
    { cookie: 'sidebar_state=banana', open: true },
  ])('reads $cookie as open=$open', ({ cookie, open }) => {
    setCookie(cookie)
    expect(readSidebarOpen()).toBe(open)
  })

  it('finds the cookie among others', () => {
    setCookie('theme=dark')
    setCookie('sidebar_state=false')
    expect(readSidebarOpen()).toBe(false)
  })
})
