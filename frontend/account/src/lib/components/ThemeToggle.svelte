<script lang="ts">
	import { onMount } from 'svelte';

	// Starts as the dark glyph (matches SSR); corrected on mount if the
	// effective theme is light.
	let dark = $state(true);

	onMount(() => {
		dark = document.documentElement.dataset.theme !== 'light';
	});

	function toggle() {
		dark = !dark;
		const theme = dark ? 'dark' : 'light';
		document.documentElement.dataset.theme = theme;
		try {
			localStorage.setItem('theme', theme);
		} catch {
			// private mode — session-only theme is fine
		}
	}
</script>

<button
	type="button"
	class="icon-btn"
	aria-label="Switch to {dark ? 'light' : 'dark'} theme"
	title="Switch to {dark ? 'light' : 'dark'} theme"
	onclick={toggle}
>
	{#if dark}
		<!-- sun (lucide) -->
		<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true">
			<circle cx="12" cy="12" r="4" />
			<path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
		</svg>
	{:else}
		<!-- moon (lucide) -->
		<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
			<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
		</svg>
	{/if}
</button>
