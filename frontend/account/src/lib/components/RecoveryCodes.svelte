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
</script>

<div class="recovery">
	<p class="warning">
		Shown once — store these safely. They will not be displayed again.
	</p>
	<div class="grid">
		{#each codes as code (code)}
			<code class="code">{code}</code>
		{/each}
	</div>
	<button type="button" class="copy" onclick={copyAll}>
		{copied ? 'Copied!' : 'Copy all'}
	</button>
	<label class="ack">
		<input type="checkbox" bind:checked={acknowledged} />
		I have saved my recovery codes
	</label>
</div>

<style>
	.recovery {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.warning {
		color: #ffb84d;
		font-size: 0.9rem;
		margin: 0;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.5rem;
	}
	.code {
		background: #1a1d24;
		border: 1px solid #2a2e38;
		border-radius: 6px;
		padding: 0.5rem 0.75rem;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.9rem;
		text-align: center;
	}
	.copy {
		align-self: flex-start;
		background: #2a2e38;
		color: #e6e8ee;
		border: 1px solid #3a3e48;
		border-radius: 6px;
		padding: 0.5rem 1rem;
		cursor: pointer;
		font-size: 0.9rem;
	}
	.copy:hover {
		background: #3a3e48;
	}
	.ack {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.9rem;
		color: #c9ccd6;
		cursor: pointer;
	}
</style>