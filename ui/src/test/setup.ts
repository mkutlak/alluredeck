import '@testing-library/jest-dom'

// localStorage / sessionStorage stub for Zustand persist middleware (jsdom compat)
function createStorageMock(): Storage {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value
    },
    removeItem: (key: string) => {
      delete store[key]
    },
    clear: () => {
      store = {}
    },
    get length() {
      return Object.keys(store).length
    },
    key: (i: number) => Object.keys(store)[i] ?? null,
  }
}

Object.defineProperty(globalThis, 'localStorage', { value: createStorageMock(), writable: true })
Object.defineProperty(globalThis, 'sessionStorage', { value: createStorageMock(), writable: true })

// Ensure window.__env__ is defined in tests
window.__env__ = {
  VITE_API_URL: 'http://localhost:5050',
  VITE_APP_TITLE: 'AllureDeck',
}

// ResizeObserver is used by cmdk and other UI libs but not available in jsdom
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// scrollIntoView is used by cmdk but not implemented in jsdom
Element.prototype.scrollIntoView = () => {}

// Radix UI Select (and other Radix primitives) call hasPointerCapture/setPointerCapture
// on pointer events; jsdom does not implement these, so stub them out.
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
}
if (!Element.prototype.setPointerCapture) {
  Element.prototype.setPointerCapture = () => {}
}
if (!Element.prototype.releasePointerCapture) {
  Element.prototype.releasePointerCapture = () => {}
}
