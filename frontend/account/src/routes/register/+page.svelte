<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, ApiError } from '../../lib/api';
	import FormField from '../../lib/components/FormField.svelte';
	import RecoveryCodes from '../../lib/components/RecoveryCodes.svelte';
	import type { RegisterResponse } from '../../lib/types';

	let username = $state('');
	let password = $state('');
	let confirm = $state('');

	let errors = $state<{ username?: string; password?: string; confirm?: string }>({});
	let submitting = $state(false);
	let serverError = $state('');
	let recoveryCodes = $state<string[] | null>(null);
	let acknowledged = $state(false);

	function validate(): boolean {
		const e: { username?: string; password?: string; confirm?: string } = {};
		if (username.length < 3 || username.length > 32) {
			e.username = 'Username must be 3-32 characters';
		} else if (!/^[a-z0-9._-]+$/i.test(username)) {
			e.username = 'Only letters, numbers, dots, underscores, hyphens';
		}
		if (password.length < 8) {
			e.password = 'Password must be at least 8 characters';
		}
		if (confirm !== password) {
			e.confirm = 'Passwords must match';
		}
		errors = e;
		return Object.keys(e).length === 0;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		serverError = '';
		if (!validate()) return;
		submitting = true;
		try {
			const res = await api<RegisterResponse>('/api/auth/register', {
				method: 'POST',
				body: { username, password }
			});
			recoveryCodes = res.recovery_codes;
		} catch (err) {
			serverError = err instanceof ApiError ? err.message : 'Something went wrong';
		} finally {
			submitting = false;
		}
	}

	function continueOnboarding() {
		goto('/onboarding');
	}
</script>

{#if recoveryCodes}
	<h1>Save your recovery codes</h1>
	<RecoveryCodes codes={recoveryCodes} bind:acknowledged />
	<button
		type="button"
		class="primary"
		disabled={!acknowledged}
		onclick={continueOnboarding}
	>
		Continue
	</button>
{:else}
	<h1>Register</h1>
	<form onsubmit={submit}>
		<FormField id="username" label="Username" bind:value={username} error={errors.username} autocomplete="username" />
		<FormField id="password" label="Password" type="password" bind:value={password} error={errors.password} autocomplete="new-password" />
		<FormField id="confirm" label="Confirm password" type="password" bind:value={confirm} error={errors.confirm} autocomplete="new-password" />
		{#if serverError}
			<p class="error" role="alert">{serverError}</p>
		{/if}
		<button type="submit" class="primary" disabled={submitting}>
			{submitting ? 'Creating…' : 'Create account'}
		</button>
	</form>
	<p class="muted">
		Already have an account? <a href="/login">Log in</a>
	</p>
{/if}

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
	.muted {
		color: #9aa0ac;
		font-size: 0.9rem;
		margin-top: 1rem;
	}
	.muted a {
		color: #5b7cff;
	}
</style>