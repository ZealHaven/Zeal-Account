<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
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

	function initial(name: string): string {
		return (name.trim()[0] ?? '?').toUpperCase();
	}
</script>

<div class="auth">
	<div class="auth-card wide">
		{#if loading}
			<p class="muted" style="margin: 0">Loading…</p>
		{:else if invalid}
			<h1>Invalid sign-in request</h1>
			<p class="error" style="margin-top: 1rem">This sign-in link is malformed or expired.</p>
			<div class="auth-foot">
				<a href="/">Go home</a>
			</div>
		{:else if error}
			<h1>Choose a persona</h1>
			<p class="error" style="margin-top: 1rem">{error}</p>
		{:else}
			<h1>Choose a persona</h1>
			<p class="sub">Sign in to <strong>{clientId}</strong> as…</p>

			{#if personas.length === 0}
				<div class="empty">
					<p>No personas yet.</p>
					<a class="btn btn-primary" href="/onboarding">Create your first persona</a>
				</div>
			{:else}
				<div class="list" role="list">
					{#each personas as p (p.id)}
						<button
							type="button"
							class="picker"
							class:selected={selectedId === p.id}
							disabled={submitting}
							onclick={() => choose(p)}
						>
							<span class="avatar">{initial(p.name)}</span>
							<span class="picker-main">
								<span class="picker-name">{p.name}</span>
								<span class="picker-identity">{p.identity ?? 'no identity'}</span>
								{#if p.description}
									<span class="picker-desc">
										{p.description.length > 120 ? p.description.slice(0, 120) + '…' : p.description}
									</span>
								{/if}
							</span>
							{#if selectedId === p.id}
								<span class="picker-check" aria-hidden="true">✓</span>
							{/if}
						</button>
					{/each}
				</div>
				{#if submitError}
					<p class="error" role="alert" style="margin-top: 1rem">{submitError}</p>
				{/if}
				<div class="auth-foot">
					<a href="/personas">Manage personas</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.list {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.picker {
		display: flex;
		align-items: center;
		gap: 0.85rem;
		width: 100%;
		text-align: left;
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		padding: 0.85rem 1rem;
		color: var(--text);
		cursor: pointer;
		font: inherit;
		transition:
			border-color var(--speed) ease,
			background-color var(--speed) ease,
			box-shadow var(--speed) ease;
	}
	.picker:hover:not(:disabled) {
		border-color: var(--primary);
		box-shadow: var(--shadow-sm);
	}
	.picker.selected {
		border-color: var(--primary);
		background: var(--primary-soft);
	}
	.picker:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.picker-main {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}
	.picker-name {
		font-weight: 600;
		font-size: 1rem;
	}
	.picker-identity {
		color: var(--text-muted);
		font-size: 0.875rem;
	}
	.picker-desc {
		color: var(--text-faint);
		font-size: 0.825rem;
		margin-top: 0.15rem;
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.picker-check {
		color: var(--primary);
		font-weight: 700;
		font-size: 1.1rem;
	}
</style>
