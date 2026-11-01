<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
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
</script>

<h1>Personas</h1>

{#if loading}
	<p>Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else}
	<div class="toolbar">
		<button type="button" class="primary" onclick={() => goto('/personas/new')}>New persona</button>
	</div>

	{#if personas.length === 0}
		<p class="muted">No personas yet.</p>
	{:else}
		<div class="list">
			{#each personas as p (p.id)}
				<a class="card" href={`/personas/${p.id}`}>
					<h2>{p.name}</h2>
					<p class="identity">{p.identity ?? 'no identity'}</p>
					{#if p.description}
						<p class="desc">{p.description}</p>
					{/if}
					<p class="meta">Created {formatDate(p.created_at)}</p>
				</a>
			{/each}
		</div>
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
	.list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.card {
		display: block;
		background: #1a1d24;
		border: 1px solid #2a2e38;
		border-radius: 8px;
		padding: 1rem;
		text-decoration: none;
		color: #e6e8ee;
	}
	.card:hover {
		border-color: #5b7cff;
	}
	.card h2 {
		margin: 0 0 0.25rem;
		font-size: 1.1rem;
	}
	.identity {
		color: #9aa0ac;
		font-size: 0.9rem;
		margin: 0 0 0.5rem;
	}
	.desc {
		font-size: 0.9rem;
		margin: 0 0 0.5rem;
		color: #c9ccd6;
	}
	.meta {
		font-size: 0.8rem;
		color: #6b7080;
		margin: 0;
	}
	.error {
		color: #ff6b6b;
	}
	.muted {
		color: #9aa0ac;
	}
</style>