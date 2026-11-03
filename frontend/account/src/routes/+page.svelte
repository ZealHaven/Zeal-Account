<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, redirectIfLoggedOut } from '../lib/api';
	import type { MeResponse } from '../lib/types';

	let loading = $state(true);
	let error = $state('');
	let data = $state<MeResponse | null>(null);

	onMount(async () => {
		try {
			data = await api<MeResponse>('/api/me');
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err, '/');
				return;
			}
			error = err instanceof ApiError ? err.message : 'Failed to load';
		} finally {
			loading = false;
		}
	});

	function initial(name: string): string {
		return (name.trim()[0] ?? '?').toUpperCase();
	}
</script>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if data}
	<div class="page-head">
		<div>
			<h1>Overview</h1>
			<p class="muted" style="margin: 0">Signed in as <strong>{data.account.username}</strong></p>
		</div>
	</div>

	<section class="card" style="margin-bottom: 1rem">
		<div class="section-head">
			<h2 style="margin: 0">Your personas</h2>
			<a class="btn btn-secondary btn-sm" href="/personas/new">New persona</a>
		</div>

		{#if data.personas.length === 0}
			<div class="empty">
				<p>No personas yet — a persona is the identity products see.</p>
				<a class="btn btn-primary" href="/onboarding">Create your first persona</a>
			</div>
		{:else}
			<div class="prow-list">
				{#each data.personas as p (p.id)}
					<a class="prow" href={`/personas/${p.id}`}>
						<span class="avatar">{initial(p.name)}</span>
						<span class="prow-main">
							<span class="prow-name">{p.name}</span>
							{#if p.identity}
								<span class="prow-identity">{p.identity}</span>
							{/if}
						</span>
						<span class="prow-chevron" aria-hidden="true">›</span>
					</a>
				{/each}
			</div>
		{/if}
	</section>

	<div class="tiles">
		<a class="tile" href="/personas">
			<span class="tile-title">Personas</span>
			<span class="tile-desc">Manage the identities products see</span>
		</a>
		<a class="tile" href="/settings">
			<span class="tile-title">Settings</span>
			<span class="tile-desc">Username, password, recovery codes</span>
		</a>
		<a class="tile" href="/notifications">
			<span class="tile-title">Notifications</span>
			<span class="tile-desc">Security &amp; identity alerts</span>
		</a>
	</div>
{/if}

<style>
	.section-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1rem;
	}

	.prow-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.prow {
		display: flex;
		align-items: center;
		gap: 0.85rem;
		padding: 0.65rem 0.75rem;
		border-radius: var(--radius-sm);
		color: var(--text);
		text-decoration: none;
		transition: background-color var(--speed) ease;
	}

	.prow:hover {
		background: var(--surface-3);
		text-decoration: none;
	}

	.prow-main {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}

	.prow-name {
		font-weight: 600;
		font-size: 0.95rem;
	}

	.prow-identity {
		color: var(--text-muted);
		font-size: 0.85rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.prow-chevron {
		color: var(--text-faint);
		font-size: 1.3rem;
		line-height: 1;
	}

	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
		gap: 0.75rem;
	}

	.tile {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 1rem 1.1rem;
		color: var(--text);
		text-decoration: none;
		box-shadow: var(--shadow-sm);
		transition:
			border-color var(--speed) ease,
			box-shadow var(--speed) ease;
	}

	.tile:hover {
		border-color: var(--border-strong);
		box-shadow: var(--shadow);
		text-decoration: none;
	}

	.tile-title {
		font-weight: 600;
		font-size: 0.95rem;
	}

	.tile-desc {
		color: var(--text-muted);
		font-size: 0.8rem;
	}
</style>
