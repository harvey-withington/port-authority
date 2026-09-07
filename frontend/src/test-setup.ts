import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/svelte'

// testing-library only auto-cleans when vitest globals are on; do it explicitly.
afterEach(() => {
  cleanup()
  try {
    localStorage.clear()
  } catch {
    // jsdom without storage: nothing to clear.
  }
})
