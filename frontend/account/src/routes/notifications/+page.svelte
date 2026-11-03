<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, redirectIfLoggedOut } from '../../lib/api';
	import type { Notification } from '../../lib/types';
	import { timeAgo } from '../../lib/timeAgo';

	let loading = $state(true);
	let error = $state('');
	let items = $state<Notification[]>([]);
	let markingAll = $state(false);

	async function load() {
		loading = true;
		error = '';
		try {
			const res = await api<{ notifications: Notification[] }>('/api/notifications?limit=100');
			items = res.notifications;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err, '/notifications');
				return;
			}
			error = err instanceof ApiError ? err.message : 'Failed to load';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function markRead(n: Notification) {
		if (n.read_at) return;
		const now = new Date().toISOString();
		items = items.map((x) => (x.id === n.id ? { ...x, read_at: now } : x));
		try {
			await api(`/api/notifications/${n.id}/read`, { method: 'POST' });
		} catch {
			// ignore
		}
	}

	async function markAll() {
		markingAll = true;
		try {
			await api('/api/notifications/read-all', { method: 'POST' });
			const now = new Date().toISOString();
			items = items.map((x) => ({ ...x, read_at: x.read_at ?? now }));
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err, '/notifications');
			}
		} finally {
			markingAll = false;
		}
	}

	let unread = $derived(items.filter((n) => !n.read_at));
	let read = $derived(items.filter((n) => n.read_at));

	function itemLink(n: Notification): string | null {
		return n.type === 'identity_claim_removed' ? '/personas' : null;
	}
</script>

<div class="page-head">
	<h1>Notifications</h1>
	<button
		type="button"
		class="btn btn-secondary"
		disabled={markingAll || unread.length === 0}
		onclick={markAll}
	>
		{markingAll ? 'Marking…' : 'Mark all read'}
	</button>
</div>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if items.length === 0}
	<div class="empty">
		<p>No notifications yet — security and identity events show up here.</p>
	</div>
{:else}
	{#if unread.length > 0}
		<h2>Unread</h2>
		<div class="stack" style="margin-bottom: 1.75rem">
			{#each unread as n (n.id)}
				<button type="button" class="notif unread" onclick={() => markRead(n)}>
					<span class="notif-top">
						<span class="notif-title">{n.title}</span>
						<span class="notif-time">{timeAgo(n.created_at)}</span>
					</span>
					<p class="notif-body">{n.body}</p>
					{#if itemLink(n)}
						<a class="notif-link" href={itemLink(n)} onclick={(e) => e.stopPropagation()}>View personas</a>
					{/if}
				</button>
			{/each}
		</div>
	{/if}

	{#if read.length > 0}
		<h2>Read</h2>
		<div class="stack">
			{#each read as n (n.id)}
				<button type="button" class="notif" onclick={() => markRead(n)}>
					<span class="notif-top">
						<span class="notif-title">{n.title}</span>
						<span class="notif-time">{timeAgo(n.created_at)}</span>
					</span>
					<p class="notif-body">{n.body}</p>
					{#if itemLink(n)}
						<a class="notif-link" href={itemLink(n)} onclick={(e) => e.stopPropagation()}>View personas</a>
					{/if}
				</button>
			{/each}
		</div>
	{/if}
{/if}

<style>
	h2 {
		font-size: 0.85rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--text-faint);
		margin: 0 0 0.75rem;
	}
	.notif-link {
		color: var(--link);
		font-size: 0.85rem;
		align-self: flex-start;
		margin-top: 0.25rem;
	}
</style>
