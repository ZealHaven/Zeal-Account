<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, ApiError, redirectIfLoggedOut } from '../../../lib/api';
	import type { Persona, PersonasResponse } from '../../../lib/types';

	let loading = $state(true);
	let invalid = $state(false);
	let error = $state('');
	let personas = $state<Persona[]>([]);
	let clientId = $state('');
	let nextUrl = $state<string | null>(null);

	let selectedId = $state<string | null>(null);
	let submitting = $state(false);
	let submitError = $state('');

	function parseClientId(n: string): string {
		try {
			const u = new URL(n, window.location.origin);
			return u.searchParams.get('client_id') ?? '';
		} catch {
			return '';
		}
	}

	onMount(async () => {
		const n = page.url.searchParams.get('next');
		if (!n || !n.startsWith('/authorize?')) {
			invalid = true;
			loading = false;
			return;
		}
		nextUrl = n;
		clientId = parseClientId(n);

		try {
			const res = await api<PersonasResponse>('/api/personas');
			personas = res.personas;
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

	async function choose(persona: Persona) {
		if (!nextUrl || submitting) return;
		selectedId = persona.id;
		submitting = true;
		submitError = '';
		try {
			const data = await api<{ next: string }>('/api/oidc/persona-select', {
				method: 'POST',
				body: { next: nextUrl, persona_id: persona.id }
			});
			window.location.assign(data.next);
		} catch (err) {
			if (err instanceof ApiError) {
				if (err.status === 401) {
					redirectIfLoggedOut(err, '/');
					return;
				}
				submitError = err.message;
			} else {
				submitError = 'Something went wrong';
			}
			selectedId = null;
		} finally {
			submitting = false;
		}
	}
</script>

{#if loading}
	<p>Loading…</p>
{:else if invalid}
	<p class="error">Invalid sign-in request.</p>
{:else if error}
	<p class="error">{error}</p>
{:else}
	<h1>Choose a persona</h1>
	<p class="subtext">Sign in to {clientId} as…</p>

	{#if personas.length === 0}
		<p class="muted">No personas yet.</p>
		<p><a href="/onboarding">Create your first persona</a></p>
	{:else}
		<div class="list" role="list">
			{#each personas as p (p.id)}
				<button
					type="button"
					class="card"
					class:selected={selectedId === p.id}
					disabled={submitting}
					onclick={() => choose(p)}
				>
					<span class="name">{p.name}</span>
					<span class="identity">{p.identity ?? 'no identity'}</span>
					{#if p.description}
						<span class="desc">{p.description.length > 120 ? p.description.slice(0, 120) + '…' : p.description}</span>
					{/if}
				</button>
			{/each}
		</div>
		{#if submitError}
			<p class="error" role="alert">{submitError}</p>
		{/if}
		<p class="manage"><a href="/personas">Manage personas</a></p>
	{/if}
{/if}

<style>
	.subtext {
		color: #9aa0ac;
		font-size: 0.95rem;
		margin: 0 0 1.25rem;
	}
	.list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		width: 100%;
		text-align: left;
		background: #1a1d24;
		border: 1px solid #2a2e38;
		border-radius: 8px;
		padding: 1rem;
		color: #e6e8ee;
		cursor: pointer;
		font: inherit;
	}
	.card:hover {
		border-color: #5b7cff;
	}
	.card.selected {
		border-color: #5b7cff;
		background: #1f2330;
	}
	.card:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.name {
		font-size: 1.1rem;
		font-weight: 600;
	}
	.identity {
		color: #9aa0ac;
		font-size: 0.9rem;
	}
	.desc {
		font-size: 0.9rem;
		color: #c9ccd6;
		margin-top: 0.25rem;
	}
	.error {
		color: #ff6b6b;
		font-size: 0.9rem;
		margin: 1rem 0 0;
	}
	.muted {
		color: #9aa0ac;
	}
	.manage {
		margin-top: 1.25rem;
		font-size: 0.9rem;
	}
	.manage a {
		color: #5b7cff;
	}
</style>