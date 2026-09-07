/// <reference types="svelte" />
/// <reference types="vite/client" />

// The Wails runtime injects `window.go` when the page runs inside the
// desktop shell. Its absence means plain-browser development.
interface Window {
  go?: unknown
}
