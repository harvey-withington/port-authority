<script lang="ts">
  import { X } from 'lucide-svelte'
  import type { DeviceClass } from '../lib/api/types'
  import { CLASS_TOKENS, SPEED_RAMP, speedColorVar, speedInkVar, type ClassToken } from '../lib/colors'
  import { linkLabel } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import ClassIcon from './ClassIcon.svelte'

  interface Props {
    onClose: () => void
  }

  let { onClose }: Props = $props()

  // Tokens that correspond to a model class; billboard/smartcard fold into these.
  const CLASS_FOR_TOKEN: Record<ClassToken, DeviceClass> = {
    storage: 'storage', video: 'video', audio: 'audio', hid: 'hid', network: 'network',
    hub: 'hub', display: 'billboard', wireless: 'wireless', printer: 'printer', serial: 'serial',
    imaging: 'imaging', composite: 'composite', vendor: 'vendor', unknown: 'unknown',
  }
  const HEALTH = ['good', 'slow', 'idle'] as const
</script>

<section class="legend" aria-label={t('legend.title')}>
  <header>
    <h3>{t('legend.title')}</h3>
    <button class="close" onclick={onClose} aria-label={t('legend.close')}><X size={14} /></button>
  </header>

  <h4>{t('legend.classes')}</h4>
  <ul class="classes">
    {#each CLASS_TOKENS as token (token)}
      <li>
        <ClassIcon cls={CLASS_FOR_TOKEN[token]} size={14} />
        <span>{t(`class.${token}`)}</span>
      </li>
    {/each}
  </ul>

  <h4>{t('legend.speeds')}</h4>
  <ul class="speeds">
    {#each SPEED_RAMP as speed (speed)}
      <li style:background={speedColorVar(speed)} style:color={speedInkVar(speed)}>{linkLabel(speed)}</li>
    {/each}
  </ul>

  <h4>{t('legend.health')}</h4>
  <ul class="health">
    {#each HEALTH as h (h)}
      <li><span class="ring ring-{h}" aria-hidden="true"></span>{t(`link.health.${h}`)}</li>
    {/each}
  </ul>
</section>

<style>
  .legend {
    padding: var(--space-3) var(--space-4);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: 0 8px 24px var(--shadow-lg);
    font-size: var(--font-size-sm);
    color: var(--text-body);
    width: min(420px, 100%);
  }
  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--space-2);
  }
  h3 {
    font-size: var(--font-size-md);
    color: var(--text-strong);
  }
  h4 {
    margin: var(--space-3) 0 var(--space-1);
    font-size: var(--font-size-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .close {
    background: none;
    border: 0;
    padding: 2px;
    color: var(--text-muted);
    display: inline-flex;
    border-radius: var(--radius-sm);
  }
  .close:hover { color: var(--text-strong); background: var(--bg-subtle-hover); }
  .classes {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 2px var(--space-3);
  }
  .classes li {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .speeds {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
  }
  .speeds li {
    padding: 3px 7px;
    border-radius: var(--radius-sm);
    font-size: var(--font-size-xs);
    font-weight: 600;
  }
  .health li {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 1px 0;
  }
  .ring {
    width: 14px;
    height: 10px;
    border-radius: 3px;
    border: 1.5px solid var(--link-idle);
  }
  .ring-good { border-color: var(--link-good); }
  .ring-slow { border-color: var(--link-slow); }
</style>
