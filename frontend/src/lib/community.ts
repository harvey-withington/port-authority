// Sharing a dock with the community knowledge base.
//
// Nothing is sent from the app: sharing opens the repository's issue form
// in the browser with the entry filled in, and a workflow there turns the
// issue into a pull request. The entry is the dock's hubs, names, ports
// and router strings; never a serial number.
import type { DockEntry, DockView } from './api/types'

export const COMMUNITY_REPO = 'https://github.com/harvey-withington/usb-device-kb'

/** The issue form's field ids, which the URL pre-fills. */
const ISSUE_TEMPLATE = 'dock.yml'

/** The entry as it should be submitted: without the local id and the local verification stamp. */
export function shareableEntry(dock: DockView): DockEntry {
  const { id: _id, source: _source, verified: _verified, ...rest } = dock
  return rest
}

/** The new-issue URL with the form filled in from the dock. */
export function shareDockUrl(dock: DockView, repo: string = COMMUNITY_REPO): string {
  const url = new URL(`${repo.replace(/\/+$/, '')}/issues/new`)
  url.searchParams.set('template', ISSUE_TEMPLATE)
  url.searchParams.set('title', `Dock: ${dock.name}`)
  url.searchParams.set('entry', JSON.stringify(shareableEntry(dock), null, 2))
  return url.toString()
}

interface WindowWithRuntime extends Window {
  runtime?: { BrowserOpenURL?: (url: string) => void }
}

/**
 * Opens a link in the system browser. Inside the desktop app the Wails
 * runtime does it, since the app window is not a browser; elsewhere a
 * new tab is.
 */
export function openExternal(url: string, win: WindowWithRuntime = window): void {
  const open = win.runtime?.BrowserOpenURL
  if (open) {
    open(url)
    return
  }
  win.open(url, '_blank', 'noopener')
}
