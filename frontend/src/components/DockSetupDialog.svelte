<script lang="ts">
  // "Which of these hubs are one dock?" The pool is every hub that belongs
  // to no known box; the finding that opened the dialog pre-ticks the ones
  // it named and suggests a name. Saving writes the user's own knowledge
  // base, and the service redraws the machine with the new box.
  import type { Device, DockEntry, Insight, LinkSpeed, Topology } from '../lib/api/types'
  import type { DeviceIndex } from '../lib/topology'
  import { deviceName, nameForId, routerName } from '../lib/topology'
  import { normalizeId } from '../lib/ids'
  import { hex4, linkDescription } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import Dialog from './Dialog.svelte'

  interface Props {
    insight: Insight
    topology: Topology | null
    index: DeviceIndex
    /** Rejects with the reason to show when the service refuses the dock. */
    onSave: (entry: DockEntry) => Promise<void>
    onClose: () => void
  }

  let { insight, topology, index, onSave, onClose }: Props = $props()

  interface Candidate {
    id: string
    device: Device
    /** "vid:pid", which is what the knowledge base records. */
    pair: string
    where: string
  }

  const candidates = $derived.by((): Candidate[] => {
    const out: Candidate[] = []
    for (const [id, ref] of index) {
      const d = ref.device
      // Root hubs are the computer's own; hubs in a known box are spoken for.
      if (!d?.hub || ref.parentId === null) continue
      if (d.enclosure && d.enclosure.kind !== 'hub') continue
      const parent = ref.parentId ? nameForId(index, ref.parentId) : null
      const where = ref.port && parent ? t('dock.setup.where', { parent, port: ref.port.number }) : ''
      out.push({ id, device: d, pair: `${hex4(d.vendor_id)}:${hex4(d.product_id)}`, where })
    }
    return out.sort((a, b) => a.pair.localeCompare(b.pair) || a.id.localeCompare(b.id))
  })

  const mentioned = $derived(new Set((insight.device_ids ?? []).map(normalizeId)))
  // A router can only be tied when its own strings are known, since that
  // is what the knowledge base matches on.
  const routers = $derived((topology?.usb4 ?? []).filter((r) => r.kind === 'device' && !r.enclosure_id && r.model))
  const suggestedRouter = $derived(routers.find((r) => mentioned.has(normalizeId(r.id))) ?? null)
  const defaultName = $derived.by((): string => {
    if (suggestedRouter) return routerName(suggestedRouter)
    const first = candidates.find((c) => mentioned.has(c.id))
    return first ? deviceName(first.device) : ''
  })

  // The user's edits sit over the defaults, so a finding can seed the form
  // without an effect and a re-render never undoes a change.
  let nameInput = $state<string | null>(null)
  let routerInput = $state<string | null>(null)
  // What the dock's cable can carry, as the user knows it from the box it
  // came in; the negotiated link is judged against this.
  let uplinkInput = $state<LinkSpeed | ''>('')
  const UPLINKS: LinkSpeed[] = ['usb4_40', 'usb4_20', 'usb4_80']
  let chosen = $state<Record<string, boolean>>({})
  let busy = $state(false)
  let error = $state<string | null>(null)

  const name = $derived(nameInput ?? defaultName)
  const routerId = $derived(routerInput ?? suggestedRouter?.id ?? '')
  const isChecked = (id: string): boolean => chosen[id] ?? mentioned.has(id)
  const toggle = (id: string, on: boolean): void => {
    chosen = { ...chosen, [id]: on }
  }

  async function save(): Promise<void> {
    const hubs = [...new Set(candidates.filter((c) => isChecked(c.id)).map((c) => c.pair))]
    if (name.trim() === '' || hubs.length === 0) {
      error = t('dock.setup.needHubs')
      return
    }
    const router = routers.find((r) => r.id === routerId)
    const entry: DockEntry = { name: name.trim(), hubs }
    if (router) {
      entry.usb4 = { vendor: router.vendor ?? '', model: router.model ?? '' }
      if (uplinkInput) entry.uplink = { kind: 'usb4', max_link: uplinkInput }
    }
    busy = true
    error = null
    try {
      await onSave(entry)
      onClose()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<Dialog title={t('dock.setup.title')} {onClose} width="580px">
  <p class="lead">{t('dock.setup.lead')}</p>

  <label class="field">
    <span class="label">{t('dock.setup.name')}</span>
    <input type="text" value={name} oninput={(e) => (nameInput = e.currentTarget.value)} disabled={busy} />
  </label>

  <fieldset class="hubs" disabled={busy}>
    <legend class="label">{t('dock.setup.hubs')}</legend>
    {#if candidates.length === 0}
      <p class="muted">{t('dock.setup.noHubs')}</p>
    {/if}
    {#each candidates as c (c.id)}
      <label class="hub">
        <input type="checkbox" checked={isChecked(c.id)} onchange={(e) => toggle(c.id, e.currentTarget.checked)} />
        <span class="hub-name">{deviceName(c.device)}</span>
        <span class="mono pair">{c.pair}</span>
        {#if c.where}<span class="muted">{c.where}</span>{/if}
      </label>
    {/each}
  </fieldset>

  {#if routers.length > 0}
    <label class="field">
      <span class="label">{t('dock.setup.router')}</span>
      <select value={routerId} onchange={(e) => (routerInput = e.currentTarget.value)} disabled={busy}>
        <option value="">{t('dock.setup.noRouter')}</option>
        {#each routers as r (r.id)}
          <option value={r.id}>{routerName(r)}</option>
        {/each}
      </select>
      <span class="hint">{t('dock.setup.routerHint')}</span>
    </label>
    {#if routerId}
      <label class="field">
        <span class="label">{t('dock.setup.uplink')}</span>
        <select value={uplinkInput} onchange={(e) => (uplinkInput = e.currentTarget.value as LinkSpeed | '')} disabled={busy}>
          <option value="">{t('dock.setup.uplink.unknown')}</option>
          {#each UPLINKS as speed (speed)}
            <option value={speed}>{linkDescription(speed)}</option>
          {/each}
        </select>
        <span class="hint">{t('dock.setup.uplinkHint')}</span>
      </label>
    {/if}
  {/if}

  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}

  {#snippet footer()}
    <button class="btn" onclick={onClose} disabled={busy}>{t('dialog.cancel')}</button>
    <button class="btn primary" onclick={save} disabled={busy}>{busy ? t('dock.setup.saving') : t('dock.setup.save')}</button>
  {/snippet}
</Dialog>

<style>
  .lead {
    color: var(--text-body);
    margin-bottom: var(--space-4);
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin-bottom: var(--space-4);
  }
  .label {
    font-size: var(--font-size-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
  }
  input[type='text'],
  select {
    font: inherit;
    padding: var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-strong);
  }
  input[type='text']:focus-visible,
  select:focus-visible {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-glow-2);
  }
  .hubs {
    margin: 0 0 var(--space-4);
    padding: 0;
    border: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .hubs legend {
    margin-bottom: var(--space-1);
  }
  .hub {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-sm);
  }
  .hub:hover {
    background: var(--bg-subtle);
  }
  .hub-name {
    font-weight: 600;
    color: var(--text-strong);
  }
  .pair {
    color: var(--text-secondary);
    font-size: var(--font-size-xs);
  }
  .muted {
    color: var(--text-muted);
    font-size: var(--font-size-sm);
  }
  .hint {
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
  .error {
    color: var(--danger);
    font-size: var(--font-size-sm);
  }
</style>
