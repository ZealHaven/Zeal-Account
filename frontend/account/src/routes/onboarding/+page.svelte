<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, ApiError } from '../../lib/api';
	import FormField from '../../lib/components/FormField.svelte';

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
			goto('/');
		} catch (err) {
			serverError = err instanceof ApiError ? err.message : 'Something went wrong';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="auth">
	<div class="auth-card">
		<h1>Create your first persona</h1>
		<p class="sub">A persona is the identity products see. You can create more later.</p>
		<form onsubmit={submit}>
			<FormField id="name" label="Name" bind:value={name} error={errors.name} />
			<FormField
				id="identity"
				label="Identity (optional)"
				bind:value={identity}
				placeholder="email, phone, handle…"
			/>
			<FormField
				id="description"
				label="Description (optional)"
				textarea
				bind:value={description}
				rows={3}
			/>
			{#if serverError}
				<p class="error" role="alert">{serverError}</p>
			{/if}
			<button type="submit" class="btn btn-primary btn-block" disabled={submitting}>
				{submitting ? 'Creating…' : 'Create persona'}
			</button>
		</form>
	</div>
</div>
