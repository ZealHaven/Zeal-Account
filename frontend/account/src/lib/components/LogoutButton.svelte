<script lang="ts">
	// Topbar sign-out. Backend: POST /api/auth/logout destroys the Redis
	// session and clears the httpOnly cookie (internal/handler/auth.go);
	// it is idempotent — a missing/expired session still returns 204.
	import { api } from '../api';

	let busy = $state(false);

	async function logout() {
		if (busy) return;
		busy = true;
		try {
			await api('/api/auth/logout', { method: 'POST' });
		} catch {
			// Network failure: the session may survive server-side, but the
			// cookie is httpOnly so the browser can do nothing more here.
			// Still navigate — the server logs the error.
		}
		// Full page load, not goto(): layouts survive client-side
		// navigation, so bell counts and loaded /api/me data from the old
		// session would leak into the next login. A reload wipes all of it.
		window.location.assign('/login');
	}
</script>

<button
	type="button"
	class="icon-btn logout"
	aria-label="Log out"
	title="Log out"
	disabled={busy}
	onclick={logout}
>
	<!-- log-out (lucide) -->
	<svg
		viewBox="0 0 24 24"
		width="17"
		height="17"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
		<polyline points="16 17 21 12 16 7" />
		<line x1="21" x2="9" y1="12" y2="12" />
	</svg>
	<span class="label">{busy ? 'Signing out…' : 'Log out'}</span>
</button>

<style>
	.logout {
		gap: 0.4rem;
		padding: 0.42rem 0.7rem;
		font-size: 0.85rem;
		font-weight: 500;
	}
	.logout:disabled {
		opacity: 0.6;
		cursor: wait;
	}
	/* Keep the icon only when the topbar gets tight. */
	@media (max-width: 480px) {
		.label {
			display: none;
		}
		.logout {
			padding: 0.42rem 0.5rem;
		}
	}
</style>
