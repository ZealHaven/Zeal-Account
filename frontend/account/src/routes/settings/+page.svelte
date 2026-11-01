<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiError, redirectIfLoggedOut } from '../../lib/api';
	import FormField from '../../lib/components/FormField.svelte';
	import RecoveryCodes from '../../lib/components/RecoveryCodes.svelte';
	import type { MeResponse } from '../../lib/types';

	let loading = $state(true);
	let loadError = $state('');

	let currentUsername = $state('');
	let newUsername = $state('');
	let usernameError = $state('');
	let usernameSaving = $state(false);
	let usernameSuccess = $state('');
	let usernameServerError = $state('');

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let passwordErrors = $state<{ current?: string; new?: string; confirm?: string }>({});
	let passwordSaving = $state(false);
	let passwordSuccess = $state('');
	let passwordServerError = $state('');

	let regenCodes = $state<string[] | null>(null);
	let regenAcknowledged = $state(false);
	let regenLoading = $state(false);
	let regenError = $state('');

	onMount(async () => {
		try {
			const res = await api<MeResponse>('/api/me');
			currentUsername = res.account.username;
			newUsername = res.account.username;
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				redirectIfLoggedOut(err, '/settings');
				return;
			}
			loadError = err instanceof ApiError ? err.message : 'Failed to load';
		} finally {
			loading = false;
		}
	});

	async function saveUsername(event: SubmitEvent) {
		event.preventDefault();
		usernameError = '';
		usernameSuccess = '';
		usernameServerError = '';
		if (!newUsername.trim()) {
			usernameError = 'Username is required';
			return;
		}
		if (newUsername.trim().length < 3 || newUsername.trim().length > 32) {
			usernameError = 'Username must be 3-32 characters';
			return;
		}
		if (!/^[a-z0-9._-]+$/i.test(newUsername.trim())) {
			usernameError = 'Only letters, numbers, dots, underscores, hyphens';
			return;
		}
		usernameSaving = true;
		try {
			await api('/api/me', {
				method: 'PATCH',
				body: { username: newUsername.trim() }
			});
			currentUsername = newUsername.trim();
			usernameSuccess = 'Username updated';
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				usernameServerError = 'Username already taken';
			} else {
				usernameServerError = err instanceof ApiError ? err.message : 'Failed to update';
			}
		} finally {
			usernameSaving = false;
		}
	}

	function validatePassword(): boolean {
		const e: { current?: string; new?: string; confirm?: string } = {};
		if (!currentPassword) e.current = 'Current password is required';
		if (newPassword.length < 8) e.new = 'Password must be at least 8 characters';
		if (confirmPassword !== newPassword) e.confirm = 'Passwords must match';
		passwordErrors = e;
		return Object.keys(e).length === 0;
	}

	async function savePassword(event: SubmitEvent) {
		event.preventDefault();
		passwordSuccess = '';
		passwordServerError = '';
		if (!validatePassword()) return;
		passwordSaving = true;
		try {
			await api('/api/me/password', {
				method: 'POST',
				body: { current_password: currentPassword, new_password: newPassword }
			});
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			passwordSuccess = 'Password updated';
		} catch (err) {
			if (err instanceof ApiError && err.status === 400) {
				passwordServerError = 'Current password is incorrect';
			} else {
				passwordServerError = err instanceof ApiError ? err.message : 'Failed to update';
			}
		} finally {
			passwordSaving = false;
		}
	}

	async function regenerate() {
		regenError = '';
		regenLoading = true;
		try {
			const res = await api<{ recovery_codes: string[] }>('/api/me/recovery-codes', {
				method: 'POST'
			});
			regenCodes = res.recovery_codes;
			regenAcknowledged = false;
		} catch (err) {
			regenError = err instanceof ApiError ? err.message : 'Failed to regenerate';
		} finally {
			regenLoading = false;
		}
	}
</script>

{#if loading}
	<p>Loading…</p>
{:else if loadError}
	<p class="error">{loadError}</p>
{:else}
	<h1>Settings</h1>

	<section>
		<h2>Username</h2>
		<p class="muted">Current: <strong>{currentUsername}</strong></p>
		<form onsubmit={saveUsername}>
			<FormField id="newUsername" label="New username" bind:value={newUsername} error={usernameError} autocomplete="username" />
			{#if usernameServerError}
				<p class="error" role="alert">{usernameServerError}</p>
			{/if}
			{#if usernameSuccess}
				<p class="success">{usernameSuccess}</p>
			{/if}
			<button type="submit" class="primary" disabled={usernameSaving}>
				{usernameSaving ? 'Saving…' : 'Update username'}
			</button>
		</form>
	</section>

	<section>
		<h2>Password</h2>
		<form onsubmit={savePassword}>
			<FormField id="currentPassword" label="Current password" type="password" bind:value={currentPassword} error={passwordErrors.current} autocomplete="current-password" />
			<FormField id="newPassword" label="New password" type="password" bind:value={newPassword} error={passwordErrors.new} autocomplete="new-password" />
			<FormField id="confirmPassword" label="Confirm new password" type="password" bind:value={confirmPassword} error={passwordErrors.confirm} autocomplete="new-password" />
			{#if passwordServerError}
				<p class="error" role="alert">{passwordServerError}</p>
			{/if}
			{#if passwordSuccess}
				<p class="success">{passwordSuccess}</p>
			{/if}
			<button type="submit" class="primary" disabled={passwordSaving}>
				{passwordSaving ? 'Saving…' : 'Update password'}
			</button>
		</form>
	</section>

	<section>
		<h2>Recovery codes</h2>
		<p class="muted">Regenerating invalidates all previous codes.</p>
		{#if regenError}
			<p class="error" role="alert">{regenError}</p>
		{/if}
		{#if regenCodes}
			<RecoveryCodes codes={regenCodes} bind:acknowledged={regenAcknowledged} />
		{:else}
			<button type="button" class="primary" disabled={regenLoading} onclick={regenerate}>
				{regenLoading ? 'Regenerating…' : 'Regenerate recovery codes'}
			</button>
		{/if}
	</section>
{/if}

<style>
	section {
		margin-bottom: 2rem;
		padding-bottom: 1.5rem;
		border-bottom: 1px solid #2a2e38;
	}
	section:last-child {
		border-bottom: none;
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
		align-self: flex-start;
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
	.muted {
		color: #9aa0ac;
		font-size: 0.9rem;
		margin-bottom: 1rem;
	}
</style>