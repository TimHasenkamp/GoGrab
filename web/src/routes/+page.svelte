<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  onMount(() => goto('/admin', { replaceState: true }));
</script>

<p style="padding:2rem;color:#71717a;font-family:Geist,ui-sans-serif,system-ui,sans-serif;">Lade …</p>
