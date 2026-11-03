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

<div class="narrow">
	{#if loading}
		<p class="muted">Loading…</p>
	{:else if loadError}
		<p class="error">{loadError}</p>
	{:else}
		<div class="page-head">
			<h1>Settings</h1>
		</div>

		<div class="stack">
			<section class="card">
				<h2>Username</h2>
				<p class="muted" style="margin-bottom: 1rem">
					Current: <strong>{currentUsername}</strong>
				</p>
				<form onsubmit={saveUsername}>
					<FormField
						id="newUsername"
						label="New username"
						bind:value={newUsername}
						error={usernameError}
						autocomplete="username"
					/>
					{#if usernameServerError}
						<p class="error" role="alert">{usernameServerError}</p>
					{/if}
					{#if usernameSuccess}
						<p class="success">{usernameSuccess}</p>
					{/if}
					<div class="btn-row" style="margin-top: 0">
						<button type="submit" class="btn btn-primary" disabled={usernameSaving}>
							{usernameSaving ? 'Saving…' : 'Update username'}
						</button>
					</div>
				</form>
			</section>

			<section class="card">
				<h2>Password</h2>
				<form onsubmit={savePassword}>
					<FormField
						id="currentPassword"
						label="Current password"
						type="password"
						bind:value={currentPassword}
						error={passwordErrors.current}
						autocomplete="current-password"
					/>
					<FormField
						id="newPassword"
						label="New password"
						type="password"
						bind:value={newPassword}
						error={passwordErrors.new}
						autocomplete="new-password"
					/>
					<FormField
						id="confirmPassword"
						label="Confirm new password"
						type="password"
						bind:value={confirmPassword}
						error={passwordErrors.confirm}
						autocomplete="new-password"
					/>
					{#if passwordServerError}
						<p class="error" role="alert">{passwordServerError}</p>
					{/if}
					{#if passwordSuccess}
						<p class="success">{passwordSuccess}</p>
					{/if}
					<div class="btn-row" style="margin-top: 0">
						<button type="submit" class="btn btn-primary" disabled={passwordSaving}>
							{passwordSaving ? 'Saving…' : 'Update password'}
						</button>
					</div>
				</form>
			</section>

			<section class="card">
				<h2>Recovery codes</h2>
				<p class="muted">Regenerating invalidates all previous codes.</p>
				{#if regenError}
					<p class="error" role="alert">{regenError}</p>
				{/if}
				{#if regenCodes}
					<RecoveryCodes codes={regenCodes} bind:acknowledged={regenAcknowledged} />
				{:else}
					<div class="btn-row" style="margin-top: 0">
						<button type="button" class="btn btn-primary" disabled={regenLoading} onclick={regenerate}>
							{regenLoading ? 'Regenerating…' : 'Regenerate recovery codes'}
						</button>
					</div>
				{/if}
			</section>
		</div>
	{/if}
</div>
