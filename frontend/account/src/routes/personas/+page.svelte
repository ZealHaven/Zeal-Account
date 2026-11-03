<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, redirectIfLoggedOut } from '../../lib/api';
	import type { PersonasResponse, Persona } from '../../lib/types';

	let loading = $state(true);
	let error = $state('');
	let personas = $state<Persona[]>([]);

	onMount(async () => {
		try {
			const res = await api<PersonasResponse>('/api/personas');
			personas = res.personas;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err, '/personas');
				return;
			}
			error = err instanceof ApiError ? err.message : 'Failed to load';
		} finally {
			loading = false;
		}
	});

	function formatDate(s: string): string {
		try {
			return new Date(s).toLocaleDateString();
		} catch {
			return s;
		}
	}

	function initial(name: string): string {
		return (name.trim()[0] ?? '?').toUpperCase();
	}
</script>

<div class="page-head">
	<h1>Personas</h1>
	<a class="btn btn-primary" href="/personas/new">New persona</a>
</div>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if personas.length === 0}
	<div class="empty">
		<p>No personas yet — a persona is the identity products see.</p>
		<a class="btn btn-primary" href="/personas/new">Create your first persona</a>
	</div>
{:else}
	<div class="stack">
		{#each personas as p (p.id)}
			<a class="card card-hover pcard" href={`/personas/${p.id}`}>
				<span class="avatar">{initial(p.name)}</span>
				<span class="pcard-main">
					<span class="pcard-name">{p.name}</span>
					<span class="pcard-identity">{p.identity ?? 'no identity'}</span>
					{#if p.description}
						<span class="pcard-desc">{p.description}</span>
					{/if}
					<span class="pcard-meta">Created {formatDate(p.created_at)}</span>
				</span>
				<span class="pcard-chevron" aria-hidden="true">›</span>
			</a>
		{/each}
	</div>
{/if}

<style>
	.pcard {
		display: flex;
		align-items: flex-start;
		gap: 0.95rem;
		padding: 1.1rem 1.2rem;
	}

	.pcard-main {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
		flex: 1;
	}

	.pcard-name {
		font-weight: 600;
		font-size: 1.05rem;
		letter-spacing: -0.01em;
	}

	.pcard-identity {
		color: var(--text-muted);
		font-size: 0.9rem;
	}

	.pcard-desc {
		color: var(--text-muted);
		font-size: 0.875rem;
		margin-top: 0.2rem;
	}

	.pcard-meta {
		color: var(--text-faint);
		font-size: 0.775rem;
		margin-top: 0.35rem;
	}

	.pcard-chevron {
		color: var(--text-faint);
		font-size: 1.4rem;
		line-height: 1;
		align-self: center;
	}
</style>
