<script lang="ts">
  import { X } from 'lucide-svelte'
  import type { Connector, DeviceClass, LinkSpeed, Port } from '../lib/api/types'
  import { CLASS_TOKENS, SPEED_RAMP, speedColorVar, speedInkVar, type ClassToken } from '../lib/colors'
  import { socketKind } from '../lib/ports'
  import { linkLabel } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import ClassIcon from './ClassIcon.svelte'
  import PortSocket from './PortSocket.svelte'

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

  function samplePort(max: LinkSpeed, connector: Connector): Port {
    return { number: 1, status: 'none', negotiated_link: 'none', max_link: max, connector }
  }
  /** One example of each socket shape, plus the bolt a USB4 socket wears. */
  const SOCKETS: { id: string; port: Port; name: string }[] = [
    { id: 'usb-c', port: samplePort('ss10', { type_c: true, user_connectable: true, multiple_companions: false }), name: 'socket.kind.usb-c' },
    { id: 'usb-a', port: samplePort('ss5', { type_c: false, user_connectable: true, multiple_companions: false }), name: 'socket.kind.usb-a' },
    { id: 'internal', port: samplePort('high', { type_c: false, user_connectable: false, multiple_companions: false }), name: 'socket.kind.internal' },
    { id: 'usb4', port: samplePort('usb4_40', { type_c: true, user_connectable: true, multiple_companions: false }), name: 'socket.usb4' },
  ]
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

  <h4>{t('legend.sockets')}</h4>
  <ul class="sockets">
    {#each SOCKETS as s (s.id)}
      <li>
        <PortSocket port={s.port} occupied={s.id !== 'internal'} size={34} />
        <span>{t(s.name)}</span>
      </li>
    {/each}
  </ul>

  <h4>{t('legend.health')}</h4>
  <ul class="health">
    {#each HEALTH as h (h)}
      <li><span class="ring ring-{h}" aria-hidden="true"></span>{t(`link.health.${h}`)}</li>
    {/each}
  </ul>

  <h4>{t('legend.diagram')}</h4>
  <ul class="diagram">
    <li>
      <svg width="48" height="16" aria-hidden="true">
        <path d="M2 4 H46" class="line good" style:stroke-width="1.5px" />
        <path d="M2 12 H46" class="line good" style:stroke-width="6px" />
      </svg>
      {t('legend.diagram.width')}
    </li>
    <li>
      <svg width="48" height="16" aria-hidden="true">
        <path d="M2 8 H46" class="line slow" style:stroke-width="4px" />
      </svg>
      {t('legend.diagram.dashed')}
    </li>
    <li>
      <svg width="48" height="16" aria-hidden="true">
        <path d="M2 8 H46" class="line good" style:stroke-width="6px" />
        <path d="M2 8 H46" class="line flow" style:stroke={speedColorVar('ss10')} style:stroke-width="3px" />
      </svg>
      {t('legend.diagram.flow')}
    </li>
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
  .sockets {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    color: var(--text-secondary);
  }
  .sockets li {
    display: flex;
    align-items: center;
    gap: 6px;
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
  .diagram li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 1px 0;
  }
  .diagram svg {
    flex-shrink: 0;
  }
  .line {
    fill: none;
    stroke-linecap: round;
  }
  .line.good { stroke: var(--link-good); opacity: 0.85; }
  .line.slow { stroke: var(--link-slow); stroke-dasharray: 6 4; }
  .line.flow { stroke-dasharray: 4 7; }
</style>
