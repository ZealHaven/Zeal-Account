<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, ApiError, redirectIfLoggedOut } from '../../../lib/api';
	import FormField from '../../../lib/components/FormField.svelte';
	import type { Persona, PersonasResponse } from '../../../lib/types';

	let id = $derived(page.params.id);

	let loading = $state(true);
	let error = $state('');
	let notFound = $state(false);
	let persona = $state<Persona | null>(null);

	let name = $state('');
	let identity = $state('');
	let description = $state('');

	let original = $state<{ name: string; identity: string; description: string } | null>(null);

	let saving = $state(false);
	let saveError = $state('');
	let saved = $state(false);
	let copied = $state(false);

	onMount(async () => {
		try {
			const res = await api<PersonasResponse>('/api/personas');
			const found = res.personas.find((p) => p.id === id);
			if (!found) {
				notFound = true;
				return;
			}
			persona = found;
			name = found.name;
			identity = found.identity ?? '';
			description = found.description ?? '';
			original = { name: found.name, identity: found.identity ?? '', description: found.description ?? '' };
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err, `/personas/${id}`);
				return;
			}
			error = err instanceof ApiError ? err.message : 'Failed to load';
		} finally {
			loading = false;
		}
	});

	async function copyId() {
		if (!persona) return;
		try {
			await navigator.clipboard.writeText(persona.id);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// ignore
		}
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!persona || !original) return;
		saveError = '';
		saved = false;

		const patch: Record<string, string | null> = {};
		if (name.trim() !== original.name) patch.name = name.trim();
		if (identity.trim() !== original.identity) patch.identity = identity.trim() || null;
		if (description.trim() !== original.description) patch.description = description.trim() || null;

		if (Object.keys(patch).length === 0) {
			saved = true;
			return;
		}

		saving = true;
		try {
			const updated = await api<Persona>(`/api/personas/${persona.id}`, {
				method: 'PATCH',
				body: patch
			});
			persona = updated;
			name = updated.name;
			identity = updated.identity ?? '';
			description = updated.description ?? '';
			original = { name: updated.name, identity: updated.identity ?? '', description: updated.description ?? '' };
			saved = true;
		} catch (err) {
			saveError = err instanceof ApiError ? err.message : 'Failed to save';
		} finally {
			saving = false;
		}
	}
</script>

{#if loading}
	<p>Loading…</p>
{:else if error}
	<p class="error">{error}</p>
{:else if notFound}
	<p class="error">Persona not found.</p>
	<p><a href="/personas">Back to personas</a></p>
{:else if persona}
	<h1>Edit persona</h1>

	<div class="id-block">
		<span class="label">Persona ID</span>
		<code class="id">{persona.id}</code>
		<button type="button" class="copy" onclick={copyId}>{copied ? 'Copied!' : 'Copy'}</button>
		<p class="hint">This is the identity products see. Read-only.</p>
	</div>

	<form onsubmit={save}>
		<FormField id="name" label="Name" bind:value={name} />
		<FormField id="identity" label="Identity" bind:value={identity} placeholder="email, phone, handle…" />
		<FormField id="description" label="Description" textarea bind:value={description} rows={3} />
		{#if saveError}
			<p class="error" role="alert">{saveError}</p>
		{/if}
		{#if saved}
			<p class="success">Saved</p>
		{/if}
		<button type="submit" class="primary" disabled={saving}>
			{saving ? 'Saving…' : 'Save changes'}
		</button>
	</form>
{/if}

<style>
	.id-block {
		background: #1a1d24;
		border: 1px solid #2a2e38;
		border-radius: 8px;
		padding: 0.75rem 1rem;
		margin-bottom: 1.5rem;
	}
	.label {
		display: block;
		font-size: 0.8rem;
		color: #9aa0ac;
		margin-bottom: 0.25rem;
	}
	.id {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.9rem;
		word-break: break-all;
	}
	.copy {
		margin-left: 0.5rem;
		background: #2a2e38;
		color: #e6e8ee;
		border: 1px solid #3a3e48;
		border-radius: 6px;
		padding: 0.25rem 0.6rem;
		cursor: pointer;
		font-size: 0.8rem;
	}
	.hint {
		font-size: 0.8rem;
		color: #6b7080;
		margin: 0.5rem 0 0;
	}
	form {
		display: flex;
		flex-direction: column;
	}
	.primary {
		background: #5b7cff;
		color: #fff;
		border: none;
		border-radius: 6px;
		padding: 0.65rem 1rem;
		font-size: 1rem;
		cursor: pointer;
		margin-top: 0.5rem;
	}
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.error {
		color: #ff6b6b;
		font-size: 0.9rem;
		margin: 0 0 1rem;
	}
	.success {
		color: #4ade80;
		font-size: 0.9rem;
		margin: 0 0 1rem;
	}
</style>