<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, ApiError } from '../../../lib/api';
	import FormField from '../../../lib/components/FormField.svelte';

	let name = $state('');
	let identity = $state('');
	let description = $state('');

	let errors = $state<{ name?: string }>({});
	let submitting = $state(false);
	let serverError = $state('');

	function validate(): boolean {
		const e: { name?: string } = {};
		if (!name.trim()) e.name = 'Name is required';
		errors = e;
		return Object.keys(e).length === 0;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		serverError = '';
		if (!validate()) return;
		submitting = true;
		try {
			await api('/api/personas', {
				method: 'POST',
				body: {
					name: name.trim(),
					identity: identity.trim() || undefined,
					description: description.trim() || undefined
				}
			});
			goto('/personas');
		} catch (err) {
			serverError = err instanceof ApiError ? err.message : 'Something went wrong';
		} finally {
			submitting = false;
		}
	}
</script>

<h1>New persona</h1>
<form onsubmit={submit}>
	<FormField id="name" label="Name" bind:value={name} error={errors.name} />
	<FormField id="identity" label="Identity (optional)" bind:value={identity} placeholder="email, phone, handle…" />
	<FormField id="description" label="Description (optional)" textarea bind:value={description} rows={3} />
	{#if serverError}
		<p class="error" role="alert">{serverError}</p>
	{/if}
	<button type="submit" class="primary" disabled={submitting}>
		{submitting ? 'Creating…' : 'Create persona'}
	</button>
</form>

<style>
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
</style>