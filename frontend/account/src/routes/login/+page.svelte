<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, ApiError } from '../../lib/api';
	import FormField from '../../lib/components/FormField.svelte';
	import type { LoginResponse } from '../../lib/types';

	let username = $state('');
	let password = $state('');
	let errors = $state<{ username?: string; password?: string }>({});
	let submitting = $state(false);
	let serverError = $state('');

	function validate(): boolean {
		const e: { username?: string; password?: string } = {};
		if (!username) e.username = 'Username is required';
		if (!password) e.password = 'Password is required';
		errors = e;
		return Object.keys(e).length === 0;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		serverError = '';
		if (!validate()) return;
		submitting = true;
		try {
			await api<LoginResponse>('/api/auth/login', {
				method: 'POST',
				body: { username, password }
			});
			const next = page.url.searchParams.get('next') || '/';
			goto(next);
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				serverError = 'Invalid username or password';
			} else {
				serverError = err instanceof ApiError ? err.message : 'Something went wrong';
			}
		} finally {
			submitting = false;
		}
	}
</script>

<h1>Log in</h1>
<form onsubmit={submit}>
	<FormField id="username" label="Username" bind:value={username} error={errors.username} autocomplete="username" />
	<FormField id="password" label="Password" type="password" bind:value={password} error={errors.password} autocomplete="current-password" />
	{#if serverError}
		<p class="error" role="alert">{serverError}</p>
	{/if}
	<button type="submit" class="primary" disabled={submitting}>
		{submitting ? 'Signing in…' : 'Log in'}
	</button>
</form>
<p class="muted">
	<a href="/forgot-password">Forgot password?</a>
</p>
<p class="muted">
	No account? <a href="/register">Register</a>
</p>

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