import '@testing-library/jest-dom'

// Ensure window.__env__ is defined in tests
window.__env__ = {
  VITE_API_URL: 'http://localhost:5050',
  VITE_APP_TITLE: 'Allure Dashboard',
}
