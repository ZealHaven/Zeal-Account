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

	// `next` is either an app route or a Go front-door route (/authorize…).
	// goto() only accepts SvelteKit routes — for anything else it throws
	// navigation_route_missing and the user would be stuck here, so those
	// get a full page load (same as the persona picker's post-select
	// redirect). Same-origin paths only: no open redirects.
	function navigateAfterLogin(next: string | null) {
		const target = next && next.startsWith('/') && !next.startsWith('//') ? next : '/';
		if (target.startsWith('/authorize')) {
			window.location.assign(target);
			return;
		}
		goto(target);
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
			navigateAfterLogin(page.url.searchParams.get('next'));
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

<div class="auth">
	<div class="auth-card">
		<h1>Log in</h1>
		<p class="sub">Sign in to your Zeal account.</p>
		<form onsubmit={submit}>
			<FormField
				id="username"
				label="Username"
				bind:value={username}
				error={errors.username}
				autocomplete="username"
			/>
			<FormField
				id="password"
				label="Password"
				type="password"
				bind:value={password}
				error={errors.password}
				autocomplete="current-password"
			/>
			{#if serverError}
				<p class="error" role="alert">{serverError}</p>
			{/if}
			<button type="submit" class="btn btn-primary btn-block" disabled={submitting}>
				{submitting ? 'Signing in…' : 'Log in'}
			</button>
		</form>
		<div class="auth-foot">
			<a href="/forgot-password">Forgot password?</a>
			<span>No account? <a href="/register">Register</a></span>
		</div>
	</div>
</div>
