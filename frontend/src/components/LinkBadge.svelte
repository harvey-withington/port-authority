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
  }

  let { negotiated, max, claimed }: Props = $props()

  const health = $derived(linkHealth({ negotiated_link: negotiated, max_link: max }, { claimed_speed: claimed }))
  const showMax = $derived(isLinkKnown(max) && max !== negotiated)
  const title = $derived(
    `${t('link.title', {
      negotiated: linkDescription(negotiated),
      max: linkDescription(max),
      claimed: linkDescription(claimed),
    })}. ${t(`link.health.${health}`)}`,
  )
</script>

<span class="link-badge health-{health}" {title}>
  <span class="speed" style:background={speedColorVar(negotiated)} style:color={speedInkVar(negotiated)}>
    {linkLabel(negotiated)}
  </span>
  {#if showMax}
    <span
      class="max"
      style:background={speedColorVar(max)}
      role="img"
      aria-label={t('link.portMax', { max: linkDescription(max) })}
    ></span>
  {/if}
</span>

<style>
  .link-badge {
    --ring: var(--link-idle);
    display: inline-flex;
    align-items: stretch;
    border: 1.5px solid var(--ring);
    border-radius: var(--radius-sm);
    overflow: hidden;
    font-size: var(--font-size-xs);
    font-weight: 600;
    line-height: 1;
    white-space: nowrap;
    vertical-align: middle;
  }
  .health-good { --ring: var(--link-good); }
  .health-slow { --ring: var(--link-slow); }
  .health-idle { --ring: var(--link-idle); }
  .speed {
    padding: 3px 6px;
  }
  .max {
    width: 7px;
    flex-shrink: 0;
  }
</style>
