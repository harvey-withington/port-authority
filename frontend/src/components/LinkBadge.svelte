<script lang="ts">
  import type { LinkSpeed } from '../lib/api/types'
  import { isLinkKnown, linkHealth } from '../lib/link'
  import { linkDescription, linkLabel } from '../lib/format'
  import { speedColorVar, speedInkVar } from '../lib/colors'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    negotiated: LinkSpeed
    max: LinkSpeed
    claimed: LinkSpeed
    /** Replaces the port sentence in the tooltip; the health verdict is still appended. */
    title?: string
  }

  let { negotiated, max, claimed, title: customTitle }: Props = $props()

  const health = $derived(linkHealth({ negotiated_link: negotiated, max_link: max }, { claimed_speed: claimed }))
  const showMax = $derived(isLinkKnown(max) && max !== negotiated)
  const sentence = $derived(customTitle ?? t('link.title', {
    negotiated: linkDescription(negotiated),
    max: linkDescription(max),
    claimed: linkDescription(claimed),
  }))
  const title = $derived(`${sentence.replace(/\.$/, '')}. ${t(`link.health.${health}`)}`)
</script>

<!-- The whole chip is the negotiated speed; the port's own ceiling, when
     it differs, is a dot beside the label rather than a second block, so
     one badge never looks like two. The ring is the health. -->
<span class="link-badge health-{health}" {title} style:background={speedColorVar(negotiated)} style:color={speedInkVar(negotiated)}>
  <span class="label">{linkLabel(negotiated)}</span>
  {#if showMax}
    <span class="max" style:background={speedColorVar(max)} role="img" aria-label={t('link.portMax', { max: linkDescription(max) })}></span>
  {/if}
</span>

<style>
  .link-badge {
    --ring: var(--link-idle);
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 2px 6px;
    border: 2px solid var(--ring);
    border-radius: var(--radius-sm);
    font-size: var(--font-size-xs);
    font-weight: 600;
    line-height: 1.2;
    white-space: nowrap;
    vertical-align: middle;
  }
  .health-good { --ring: var(--link-good); }
  .health-slow { --ring: var(--link-slow); }
  .health-idle { --ring: var(--link-idle); }
  /* Outlined in the label's ink so it stands out even when the port's
     colour is a neighbour of the link's. */
  .max {
    width: 8px;
    height: 8px;
    flex-shrink: 0;
    border-radius: 50%;
    box-shadow: 0 0 0 1px currentColor;
  }
</style>
