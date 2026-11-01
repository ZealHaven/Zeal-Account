<script lang="ts">
	import { page } from '$app/state';
	import NotificationBell from '../lib/components/NotificationBell.svelte';

	let { children } = $props();

	let pathname = $derived(page.url.pathname);
	let hideBell = $derived(
		pathname === '/login' || pathname === '/register' || pathname === '/forgot-password'
	);
</script>

<header class="topbar">
	<a href="/" class="wordmark">Zeal Account</a>
	{#if !hideBell}
		<NotificationBell />
	{/if}
</header>

<main>
	{@render children()}
</main>

<style>
	:global(body) {
		margin: 0;
		font-family:
			system-ui,
			-apple-system,
			sans-serif;
		background: #0f1115;
		color: #e6e8ee;
	}

	.topbar {
		max-width: 36rem;
		margin: 0 auto;
		padding: 1.25rem 1.5rem 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.wordmark {
		color: #e6e8ee;
		text-decoration: none;
		font-weight: 600;
		font-size: 1.05rem;
	}

	.wordmark:hover {
		color: #5b7cff;
	}

	main {
		max-width: 36rem;
		margin: 0 auto;
		padding: 1.5rem 1.5rem 3rem;
	}
</style>