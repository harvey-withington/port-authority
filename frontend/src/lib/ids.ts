// Device ids are Windows PnP instance ids. They are case-insensitive by
// convention and the throughput stream upper-cases them, so every lookup
// goes through normalizeId.

export function normalizeId(id: string): string {
  return id.trim().toUpperCase()
}

export function sameId(a: string | null | undefined, b: string | null | undefined): boolean {
  if (!a || !b) return false
  return normalizeId(a) === normalizeId(b)
}

/** Sorted, normalised device ids joined into a stable key. */
export function idSetKey(ids: readonly string[] | undefined): string {
  return (ids ?? []).map(normalizeId).sort().join('|')
}
