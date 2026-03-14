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

// ResizeObserver is used by cmdk and other UI libs but not available in jsdom.
// Recharts' ResponsiveContainer relies on ResizeObserver to get dimensions — call
// the callback immediately with a non-zero rect so charts render without warnings.
// NOTE: This mock calls the callback synchronously on observe(), unlike real browsers
// which batch and fire callbacks asynchronously. This is acceptable for smoke tests.
// For tests that verify resize-dependent behavior, consider using @juggle/resize-observer.
globalThis.ResizeObserver = class ResizeObserver {
  constructor(private callback: ResizeObserverCallback) {}
  observe(_target: Element) {
    this.callback(
      [{ contentRect: { width: 100, height: 100 } } as unknown as ResizeObserverEntry],
      this,
    )
  }
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
