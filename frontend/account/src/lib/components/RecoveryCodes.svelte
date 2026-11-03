<script lang="ts">
	let {
		codes,
		acknowledged = $bindable(false)
	}: {
		codes: string[];
		acknowledged?: boolean;
	} = $props();

	let copied = $state(false);

	async function copyAll() {
		try {
			await navigator.clipboard.writeText(codes.join('\n'));
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// ignore
		}
	}

	function download() {
		const stamp = new Date().toISOString().slice(0, 10);
		const content =
			`Zeal Account recovery codes (${stamp})\n` +
			`Keep these safe — each code works once to reset your password.\n\n` +
			codes.join('\n') +
			'\n';
		const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `zeal-account-recovery-codes-${stamp}.txt`;
		document.body.appendChild(a);
		a.click();
		a.remove();
		URL.revokeObjectURL(url);
	}
</script>

<div class="recovery card">
	<p class="warning">
		Shown once — store these safely. They will not be displayed again.
	</p>
	<div class="grid">
		{#each codes as code (code)}
			<code class="code">{code}</code>
		{/each}
	</div>
	<div class="actions">
		<button type="button" class="btn btn-secondary btn-sm" onclick={download}>
			<!-- download (lucide) -->
			<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
				<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
				<polyline points="7 10 12 15 17 10" />
				<line x1="12" x2="12" y1="15" y2="3" />
			</svg>
			Download .txt
		</button>
		<button type="button" class="btn btn-secondary btn-sm" onclick={copyAll}>
			<!-- copy (lucide) -->
			<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
				<rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
				<path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
			</svg>
			{copied ? 'Copied!' : 'Copy all'}
		</button>
	</div>
	<label class="ack">
		<input type="checkbox" bind:checked={acknowledged} />
		I have saved my recovery codes
	</label>
</div>

<style>
	.recovery {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
		padding: 1.25rem;
	}
	.warning {
		color: var(--warning);
		font-size: 0.875rem;
		margin: 0;
		font-weight: 500;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.5rem;
	}
	.code {
		background: var(--code-bg);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 0.5rem 0.75rem;
		font-family: var(--font-mono);
		font-size: 0.9rem;
		letter-spacing: 0.02em;
		text-align: center;
		color: var(--text);
	}
	.actions {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.ack {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.9rem;
		color: var(--text-muted);
		cursor: pointer;
	}
	.ack input {
		accent-color: var(--primary);
		width: 1rem;
		height: 1rem;
		cursor: pointer;
	}
</style>
