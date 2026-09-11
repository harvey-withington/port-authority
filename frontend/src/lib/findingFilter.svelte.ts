// Which device the findings panel is narrowed to.
//
// The flag on a device is a question, "what was found about this?", and
// the answer is the findings panel showing only what names that device.
// The filter carries the ids the flag stood for (one device, or every
// flagged member of a folded dock) and a name to print on the control,
// because a dock is a box rather than a device id. The token increments per
// request so asking for the same device twice still draws the eye to the
// cards. Not persisted: it answers a click, it is not a setting.
import { layout } from './layout.svelte'

export interface FindingFilter {
  ids: readonly string[]
  name: string
  token: number
}

let current = $state<FindingFilter | null>(null)
let token = 0

export const findingFilter = {
  get current(): FindingFilter | null {
    return current
  },
  /** Narrows the panel to the findings naming these devices, and makes sure the panel is showing. */
  show(ids: readonly string[], name: string): void {
    current = { ids: [...ids], name, token: ++token }
    layout.openPanels()
  },
  clear(): void {
    current = null
  },
}
