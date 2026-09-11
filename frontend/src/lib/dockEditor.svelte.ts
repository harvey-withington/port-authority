// What the dock dialogs are doing: setting a dock up from a finding, or
// asking before forgetting one. One store, because a finding card and a
// box in the diagram can both start it, and only one dialog is ever open.
import type { Enclosure, Insight } from './api/types'

export type DockEditorState =
  | { kind: 'idle' }
  | { kind: 'setup'; insight: Insight }
  | { kind: 'forget'; enclosure: Enclosure }

let current = $state<DockEditorState>({ kind: 'idle' })

export const dockEditor = {
  get state(): DockEditorState {
    return current
  },
  /** Opens the setup dialog seeded from a "dock not recognised" finding. */
  setup(insight: Insight): void {
    current = { kind: 'setup', insight }
  },
  /** Asks before forgetting one of the user's own docks. */
  forget(enclosure: Enclosure): void {
    current = { kind: 'forget', enclosure }
  },
  close(): void {
    current = { kind: 'idle' }
  },
}
