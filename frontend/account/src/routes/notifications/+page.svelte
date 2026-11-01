<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
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

<h1>Notifications</h1>

<div class="toolbar">
	<button
		type="button"
		class="primary"
		disabled={markingAll || unread.length === 0}
		onclick={markAll}
	>
		{markingAll ? 'Marking…' : 'Mark all read'}
	</button>
</div>

{#if loading}
	<p>Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if items.length === 0}
	<p class="muted">No notifications yet.</p>
{:else}
	{#if unread.length > 0}
		<h2>Unread</h2>
		<ul class="list">
			{#each unread as n (n.id)}
				<li>
					<button
						type="button"
						class="item unread"
						onclick={() => markRead(n)}
					>
						<div class="item-top">
							<span class="item-title">{n.title}</span>
							<span class="item-time">{timeAgo(n.created_at)}</span>
						</div>
						<p class="item-body">{n.body}</p>
						{#if itemLink(n)}
							<a class="item-link" href={itemLink(n)} onclick={(e) => e.stopPropagation()}>View personas</a>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}

	{#if read.length > 0}
		<h2>Read</h2>
		<ul class="list">
			{#each read as n (n.id)}
				<li>
					<button type="button" class="item" onclick={() => markRead(n)}>
						<div class="item-top">
							<span class="item-title">{n.title}</span>
							<span class="item-time">{timeAgo(n.created_at)}</span>
						</div>
						<p class="item-body">{n.body}</p>
						{#if itemLink(n)}
							<a class="item-link" href={itemLink(n)} onclick={(e) => e.stopPropagation()}>View personas</a>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
{/if}

<style>
	.toolbar {
		margin-bottom: 1rem;
	}
	.primary {
		background: #5b7cff;
		color: #fff;
		border: none;
		border-radius: 6px;
		padding: 0.6rem 1rem;
		font-size: 0.95rem;
		cursor: pointer;
	}
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.list {
		list-style: none;
		padding: 0;
		margin: 0 0 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.item {
		width: 100%;
		text-align: left;
		background: #1a1d24;
		border: 1px solid #2a2e38;
		color: #e6e8ee;
		padding: 0.75rem 1rem;
		border-radius: 8px;
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.item:hover {
		border-color: #5b7cff;
	}
	.item.unread {
		background: #1f2330;
		border-color: #3a3e48;
	}
	.item-top {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		align-items: baseline;
	}
	.item-title {
		font-size: 1rem;
		font-weight: 600;
	}
	.item-time {
		font-size: 0.75rem;
		color: #6b7080;
		white-space: nowrap;
	}
	.item-body {
		font-size: 0.9rem;
		color: #c9ccd6;
		margin: 0;
	}
	.item-link {
		color: #5b7cff;
		font-size: 0.85rem;
		text-decoration: none;
		align-self: flex-start;
		margin-top: 0.25rem;
	}
	h2 {
		font-size: 1rem;
		color: #9aa0ac;
		margin: 1rem 0 0.5rem;
	}
	.error {
		color: #ff6b6b;
	}
	.muted {
		color: #9aa0ac;
	}
</style>