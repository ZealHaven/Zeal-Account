<script lang="ts">
	import { page } from '$app/state';
	import '../app.css';
	import NotificationBell from '../lib/components/NotificationBell.svelte';
	import ThemeToggle from '../lib/components/ThemeToggle.svelte';

	let { children } = $props();

	let pathname = $derived(page.url.pathname);
	let hideBell = $derived(
		pathname === '/login' || pathname === '/register' || pathname === '/forgot-password'
	);
</script>

<header class="topbar">
	<div class="topbar-inner">
		<a href="/" class="wordmark">Zeal Account</a>
		<div class="topbar-actions">
			<ThemeToggle />
			{#if !hideBell}
				<NotificationBell />
			{/if}
		</div>
	</div>
</header>

<main class="container">
	{@render children()}
</main>
