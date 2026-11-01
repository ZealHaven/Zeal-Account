<script lang="ts">
	import { api, ApiError } from '../../lib/api';
	import FormField from '../../lib/components/FormField.svelte';

	let step = $state(1);
	let username = $state('');
	let recoveryCode = $state('');
	let newPassword = $state('');
	let confirm = $state('');

	let errors = $state<{ username?: string; recoveryCode?: string; newPassword?: string; confirm?: string }>({});
	let submitting = $state(false);
	let serverError = $state('');
	let success = $state(false);

	function validateStep1(): boolean {
		const e: { username?: string } = {};
		if (!username) e.username = 'Username is required';
		errors = e;
		return Object.keys(e).length === 0;
	}

	function validateStep2(): boolean {
		const e: { recoveryCode?: string; newPassword?: string; confirm?: string } = {};
		if (!recoveryCode) e.recoveryCode = 'Recovery code is required';
		if (newPassword.length < 8) e.newPassword = 'Password must be at least 8 characters';
		if (confirm !== newPassword) e.confirm = 'Passwords must match';
		errors = e;
		return Object.keys(e).length === 0;
	}

	async function submitStep1(event: SubmitEvent) {
		event.preventDefault();
		serverError = '';
		if (!validateStep1()) return;
		submitting = true;
		try {
			await api('/api/auth/forgot-password', {
				method: 'POST',
				body: { username }
			});
			step = 2;
		} catch (err) {
			serverError = err instanceof ApiError ? err.message : 'Something went wrong';
		} finally {
			submitting = false;
		}
	}

	async function submitStep2(event: SubmitEvent) {
		event.preventDefault();
		serverError = '';
		if (!validateStep2()) return;
		submitting = true;
		try {
			await api('/api/auth/reset-password', {
				method: 'POST',
				body: { username, recovery_code: recoveryCode, new_password: newPassword }
			});
			success = true;
		} catch (err) {
			if (err instanceof ApiError && err.status === 400) {
				serverError = 'Invalid or expired recovery code';
			} else {
				serverError = err instanceof ApiError ? err.message : 'Something went wrong';
			}
		} finally {
			submitting = false;
		}
	}
</script>

<h1>Forgot password</h1>

{#if success}
	<p class="success">Password reset. You can now log in.</p>
	<p class="muted"><a href="/login">Go to login</a></p>
{:else if step === 1}
	<form onsubmit={submitStep1}>
		<FormField id="username" label="Username" bind:value={username} error={errors.username} autocomplete="username" />
		{#if serverError}
			<p class="error" role="alert">{serverError}</p>
		{/if}
		<button type="submit" class="primary" disabled={submitting}>
			{submitting ? 'Sending…' : 'Continue'}
		</button>
	</form>
{:else}
	<form onsubmit={submitStep2}>
		<FormField id="recoveryCode" label="Recovery code" bind:value={recoveryCode} error={errors.recoveryCode} />
		<FormField id="newPassword" label="New password" type="password" bind:value={newPassword} error={errors.newPassword} autocomplete="new-password" />
		<FormField id="confirm" label="Confirm new password" type="password" bind:value={confirm} error={errors.confirm} autocomplete="new-password" />
		{#if serverError}
			<p class="error" role="alert">{serverError}</p>
		{/if}
		<button type="submit" class="primary" disabled={submitting}>
			{submitting ? 'Resetting…' : 'Reset password'}
		</button>
	</form>
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
	.success {
		color: #4ade80;
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