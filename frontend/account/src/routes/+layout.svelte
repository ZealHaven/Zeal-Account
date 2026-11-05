<script lang="ts">
	import { page } from '$app/state';
	import '../app.css';
	import LogoutButton from '../lib/components/LogoutButton.svelte';
	import NotificationBell from '../lib/components/NotificationBell.svelte';
	import ThemeToggle from '../lib/components/ThemeToggle.svelte';

	let { children } = $props();

	let pathname = $derived(page.url.pathname);
	// Session controls (bell, log out) are pointless on the auth pages
	// themselves — you're signed out there by definition.
	let hideSessionUi = $derived(
		pathname === '/login' || pathname === '/register' || pathname === '/forgot-password'
	);
</script>

<header class="topbar">
	<div class="topbar-inner">
		<a href="/" class="wordmark">Zeal Account</a>
		<div class="topbar-actions">
			<ThemeToggle />
			{#if !hideSessionUi}
				<NotificationBell />
				<LogoutButton />
			{/if}
		</div>
	</div>
</header>

<main class="container">
	{@render children()}
</main>
