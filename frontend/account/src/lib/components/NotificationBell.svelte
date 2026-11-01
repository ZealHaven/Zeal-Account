<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, ApiError, redirectIfLoggedOut } from '../api';
	import type { Notification } from '../types';
	import { timeAgo } from '../timeAgo';

	let open = $state(false);
	let count = $state(0);
	let items = $state<Notification[]>([]);
	let loading = $state(false);
	let error = $state('');
	let markingAll = $state(false);
	let container: HTMLElement | undefined = $state();

	async function fetchCount() {
		try {
			const res = await api<{ unread: number }>('/api/notifications/unread-count');
			count = res.unread;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err);
			}
		}
	}

	async function fetchList() {
		loading = true;
		error = '';
		try {
			const res = await api<{ notifications: Notification[] }>('/api/notifications?limit=10');
			items = res.notifications;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err);
				return;
			}
			error = err instanceof ApiError ? err.message : 'Failed to load';
		} finally {
			loading = false;
		}
	}

	function toggle() {
		open = !open;
		if (open && items.length === 0 && !loading) {
			fetchList();
		}
	}

	async function markRead(n: Notification) {
		if (n.read_at) return;
		const now = new Date().toISOString();
		items = items.map((x) => (x.id === n.id ? { ...x, read_at: now } : x));
		count = Math.max(0, count - 1);
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
			count = 0;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err);
			}
		} finally {
			markingAll = false;
		}
	}

	function onItemClick(n: Notification) {
		if (!n.read_at) markRead(n);
		if (n.type === 'identity_claim_removed') {
			open = false;
			goto('/personas');
		} else {
			open = false;
		}
	}

	onMount(() => {
		fetchCount();
		const onClick = (e: MouseEvent) => {
			if (container && !container.contains(e.target as Node)) {
				open = false;
			}
		};
		window.addEventListener('click', onClick);
		return () => window.removeEventListener('click', onClick);
	});

	let badgeLabel = $derived(count > 9 ? '9+' : String(count));
</script>

<div class="bell-wrap" bind:this={container}>
	<button
		type="button"
		class="bell"
		aria-label="Notifications"
		aria-expanded={open}
		onclick={toggle}
	>
		<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true">
			<path
				d="M12 3a6 6 0 0 0-6 6v3.5L4.5 15h15L18 12.5V9a6 6 0 0 0-6-6Zm0 18a2.5 2.5 0 0 0 2.45-2h-4.9A2.5 2.5 0 0 0 12 21Z"
				fill="currentColor"
			/>
		</svg>
		{#if count > 0}
			<span class="badge">{badgeLabel}</span>
		{/if}
	</button>

	{#if open}
		<div class="panel" role="menu" aria-label="Notifications">
			<div class="panel-header">
				<span class="panel-title">Notifications</span>
				<button
					type="button"
					class="link-btn"
					disabled={markingAll || items.every((n) => n.read_at)}
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
				<p class="muted">No notifications yet.</p>
			{:else}
				<ul class="list">
					{#each items as n (n.id)}
						<li>
							<button
								type="button"
								class="item"
								class:unread={!n.read_at}
								onclick={() => onItemClick(n)}
							>
								<div class="item-top">
									<span class="item-title">{n.title}</span>
									<span class="item-time">{timeAgo(n.created_at)}</span>
								</div>
								<p class="item-body">{n.body}</p>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
			<div class="panel-footer">
				<a href="/notifications" onclick={() => (open = false)}>View all</a>
			</div>
		</div>
	{/if}
</div>

<style>
	.bell-wrap {
		position: relative;
	}
	.bell {
		background: #2a2e38;
		color: #e6e8ee;
		border: 1px solid #3a3e48;
		border-radius: 6px;
		padding: 0.4rem 0.5rem;
		cursor: pointer;
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.bell:hover {
		background: #3a3e48;
	}
	.badge {
		position: absolute;
		top: -6px;
		right: -6px;
		background: #ff6b6b;
		color: #fff;
		font-size: 0.7rem;
		font-weight: 600;
		min-width: 16px;
		height: 16px;
		padding: 0 4px;
		border-radius: 8px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.panel {
		position: absolute;
		right: 0;
		top: calc(100% + 0.5rem);
		width: 20rem;
		max-width: 90vw;
		background: #1a1d24;
		border: 1px solid #2a2e38;
		border-radius: 8px;
		padding: 0.75rem;
		z-index: 50;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
	}
	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.5rem;
	}
	.panel-title {
		font-weight: 600;
		font-size: 0.95rem;
	}
	.link-btn {
		background: none;
		border: none;
		color: #5b7cff;
		cursor: pointer;
		font-size: 0.8rem;
		padding: 0;
	}
	.link-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		max-height: 24rem;
		overflow-y: auto;
	}
	.item {
		width: 100%;
		text-align: left;
		background: transparent;
		border: none;
		color: #e6e8ee;
		padding: 0.5rem;
		border-radius: 6px;
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.item:hover {
		background: #2a2e38;
	}
	.item.unread {
		background: #1f2330;
	}
	.item-top {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		align-items: baseline;
	}
	.item-title {
		font-size: 0.9rem;
		font-weight: 600;
	}
	.item-time {
		font-size: 0.75rem;
		color: #6b7080;
		white-space: nowrap;
	}
	.item-body {
		font-size: 0.8rem;
		color: #c9ccd6;
		margin: 0;
	}
	.muted {
		color: #9aa0ac;
		font-size: 0.9rem;
		margin: 0.5rem 0;
	}
	.error {
		color: #ff6b6b;
		font-size: 0.9rem;
		margin: 0.5rem 0;
	}
	.panel-footer {
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid #2a2e38;
		text-align: right;
	}
	.panel-footer a {
		color: #5b7cff;
		font-size: 0.85rem;
		text-decoration: none;
	}
</style>