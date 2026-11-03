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
		class="icon-btn bell"
		aria-label="Notifications"
		aria-expanded={open}
		onclick={toggle}
	>
		<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
			<path d="M10.268 21a2 2 0 0 0 3.464 0" />
			<path d="M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326" />
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
				<p class="muted" style="margin: 0.5rem 0">Loading…</p>
			{:else if error}
				<p class="error" style="margin: 0.5rem 0; font-size: 0.85rem">{error}</p>
			{:else if items.length === 0}
				<p class="muted" style="margin: 0.5rem 0">No notifications yet.</p>
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
								<span class="item-top">
									<span class="item-title">{n.title}</span>
									<span class="item-time">{timeAgo(n.created_at)}</span>
								</span>
								<span class="item-body">{n.body}</span>
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
		padding: 0.45rem 0.55rem;
	}
	.badge {
		position: absolute;
		top: -6px;
		right: -6px;
		background: var(--error);
		color: #fff;
		font-size: 0.68rem;
		font-weight: 700;
		min-width: 16px;
		height: 16px;
		padding: 0 4px;
		border-radius: 8px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border: 2px solid var(--bg);
	}
	.panel {
		position: absolute;
		right: 0;
		top: calc(100% + 0.6rem);
		width: 21rem;
		max-width: 90vw;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 0.85rem;
		z-index: 50;
		box-shadow: var(--shadow-lg);
	}
	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.6rem;
	}
	.panel-title {
		font-weight: 600;
		font-size: 0.95rem;
	}
	.link-btn {
		background: none;
		border: none;
		color: var(--link);
		cursor: pointer;
		font-size: 0.8rem;
		padding: 0;
		font-family: inherit;
	}
	.link-btn:hover {
		text-decoration: underline;
	}
	.link-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
		text-decoration: none;
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
		color: var(--text);
		padding: 0.55rem 0.6rem;
		border-radius: var(--radius-sm);
		cursor: pointer;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font: inherit;
		transition: background-color var(--speed) ease;
	}
	.item:hover {
		background: var(--surface-3);
	}
	.item.unread {
		background: var(--primary-soft);
	}
	.item-top {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		align-items: baseline;
	}
	.item-title {
		font-size: 0.875rem;
		font-weight: 600;
	}
	.item-time {
		font-size: 0.72rem;
		color: var(--text-faint);
		white-space: nowrap;
	}
	.item-body {
		font-size: 0.8rem;
		color: var(--text-muted);
		margin: 0;
	}
	.panel-footer {
		margin-top: 0.6rem;
		padding-top: 0.6rem;
		background: linear-gradient(90deg, transparent, var(--border) 12%, var(--border) 88%, transparent)
			top / 100% 1px no-repeat;
		text-align: right;
	}
	.panel-footer a {
		color: var(--link);
		font-size: 0.85rem;
		text-decoration: none;
	}
	.panel-footer a:hover {
		text-decoration: underline;
	}
</style>
