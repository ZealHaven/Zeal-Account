<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
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
</script>

{#if loading}
	<p>Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if data}
	<h1>Zeal Account</h1>
	<p class="muted">Signed in as <strong>{data.account.username}</strong></p>

	<nav class="links">
		<a href="/personas">Personas</a>
		<a href="/settings">Settings</a>
	</nav>

	<h2>Your personas</h2>
	{#if data.personas.length === 0}
		<p class="muted">No personas yet. <a href="/onboarding">Create your first</a>.</p>
	{:else}
		<ul class="personas">
			{#each data.personas as p (p.id)}
				<li>
					<a href={`/personas/${p.id}`}>{p.name}</a>
					{#if p.identity}
						<span class="muted">· {p.identity}</span>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
{/if}

<style>
	.muted {
		color: #9aa0ac;
	}
	.error {
		color: #ff6b6b;
	}
	.links {
		display: flex;
		gap: 1rem;
		margin: 1rem 0 1.5rem;
	}
	.links a {
		color: #5b7cff;
		text-decoration: none;
	}
	.personas {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.personas a {
		color: #e6e8ee;
		text-decoration: none;
	}
	.personas a:hover {
		text-decoration: underline;
	}
</style>