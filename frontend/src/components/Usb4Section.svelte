<script lang="ts">
  import { HardDrive, Zap } from 'lucide-svelte'
  import type { USB4Router } from '../lib/api/types'
  import { USB4_ROUTER_SPEED, speedColorVar } from '../lib/colors'
  import { linkLabel } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import { routerName } from '../lib/topology'

  interface Props {
    routers: USB4Router[]
  }

  let { routers }: Props = $props()

  const color = speedColorVar(USB4_ROUTER_SPEED)
</script>

<section class="usb4">
  <header>
    <span class="icon" style:color={color} aria-hidden="true"><Zap size={16} /></span>
    <h3>{t('tree.usb4.title')}</h3>
    <span class="speed" style:color={color}>{linkLabel(USB4_ROUTER_SPEED)}</span>
  </header>
  {#if routers.length === 0}
    <p class="empty">{t('tree.usb4.empty')}</p>
  {:else}
    <ul>
      {#each routers as router (router.id)}
        <li class="router" style:--indent={router.depth} style:--class-color={color}>
          <div class="row">
            <span class="icon" style:color={color} aria-hidden="true"><Zap size={14} /></span>
            <div class="main">
              <div class="name">{routerName(router)}</div>
              <div class="meta">
                <span>{t(`tree.usb4.${router.kind}`)}</span>
                <span class="mono">{router.vendor_id.toString(16).padStart(4, '0')}:{router.product_id.toString(16).padStart(4, '0')}</span>
              </div>
              {#if router.children?.length}
                <ul class="children" aria-label={t('tree.usb4.children')}>
                  {#each router.children as child (child)}
                    <li><HardDrive size={13} aria-hidden="true" /><span>{child}</span></li>
                  {/each}
                </ul>
              {/if}
            </div>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .usb4 {
    padding: var(--space-2) 0;
    border-top: 1px solid var(--border-muted);
    margin-top: var(--space-2);
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
  }
  h3 {
    font-size: var(--font-size-md);
    font-weight: 600;
    color: var(--text-primary);
  }
  .speed {
    font-size: var(--font-size-xs);
    font-weight: 600;
  }
  .icon {
    display: inline-flex;
  }
  .empty {
    padding: 0 var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-sm);
  }
  .router {
    margin-left: calc(var(--indent, 0) * var(--tree-indent) + var(--space-2));
  }
  .row {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2) var(--space-1) var(--space-1);
    border-left: var(--stripe-width) solid var(--class-color);
    border-radius: var(--radius-sm);
  }
  .name {
    font-weight: 600;
    color: var(--text-strong);
  }
  .meta {
    display: flex;
    gap: var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
  .children {
    margin-top: var(--space-1);
    font-size: var(--font-size-sm);
    color: var(--text-body);
  }
  .children li {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--class-storage);
  }
  .children li span {
    color: var(--text-body);
  }
</style>
