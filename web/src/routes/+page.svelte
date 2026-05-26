<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  onMount(() => goto('/admin', { replaceState: true }));
</script>

<p style="padding:2rem;color:#64748b;font-family:system-ui;">Lade …</p>
