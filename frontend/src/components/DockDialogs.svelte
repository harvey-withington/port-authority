<script lang="ts">
  // Hosts the dock dialogs the editor store asks for, and does the talking
  // to the service, so the cards and boxes that open them stay simple.
  import type { LiveStore } from '../lib/live.svelte'
  import { dockEditor } from '../lib/dockEditor.svelte'
  import { t } from '../lib/i18n.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import DockSetupDialog from './DockSetupDialog.svelte'

  interface Props {
    live: LiveStore
  }

  let { live }: Props = $props()

  const current = $derived(dockEditor.state)
  let busy = $state(false)
  let error = $state<string | null>(null)

  async function forget(dockId: string): Promise<void> {
    busy = true
    error = null
    try {
      await live.forgetDock(dockId)
      dockEditor.close()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  function close(): void {
    error = null
    dockEditor.close()
  }
</script>

{#if current.kind === 'setup'}
  <DockSetupDialog
    insight={current.insight}
    topology={live.topology}
    index={live.index}
    onSave={async (entry) => {
      await live.addDock(entry)
    }}
    onClose={close}
  />
{:else if current.kind === 'forget'}
  <ConfirmDialog
    title={t('dock.forget.title', { name: current.enclosure.name ?? '' })}
    body={t('dock.forget.body')}
    confirmLabel={t('dock.forget.confirm')}
    danger
    {busy}
    {error}
    onConfirm={() => forget(current.kind === 'forget' ? (current.enclosure.dock_id ?? '') : '')}
    onCancel={close}
  />
{/if}
