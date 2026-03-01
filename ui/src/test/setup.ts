import '@testing-library/jest-dom'

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
