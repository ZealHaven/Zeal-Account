<script lang="ts">
	let {
		label,
		id,
		name = id,
		type = 'text',
		value = $bindable(''),
		error = '',
		placeholder = '',
		required = false,
		disabled = false,
		autocomplete,
		rows = 3,
		textarea = false
	}: {
		label: string;
		id: string;
		name?: string;
		type?: string;
		value?: string;
		error?: string;
		placeholder?: string;
		required?: boolean;
		disabled?: boolean;
		autocomplete?: string;
		rows?: number;
		textarea?: boolean;
	} = $props();

	let revealed = $state(false);
	let isPassword = $derived(type === 'password' && !textarea);
</script>

<div class="field">
	<label class="label" for={id}>{label}</label>
	{#if textarea}
		<textarea
			{id}
			{name}
			bind:value
			{placeholder}
			{required}
			{disabled}
			{rows}
			aria-invalid={error ? 'true' : undefined}
		></textarea>
	{:else if isPassword}
		<div class="input-wrap">
			<input
				{id}
				{name}
				type={revealed ? 'text' : 'password'}
				bind:value
				{placeholder}
				{required}
				{disabled}
				autocomplete={autocomplete as any}
				aria-invalid={error ? 'true' : undefined}
			/>
			<button
				type="button"
				class="eye"
				aria-label={revealed ? 'Hide password' : 'Show password'}
				aria-pressed={revealed}
				onclick={() => (revealed = !revealed)}
			>
				{#if revealed}
					<!-- eye-off (lucide) -->
					<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68" />
						<path d="M6.61 6.61A13.53 13.53 0 0 0 2 12s3.5 7 10 7a9.74 9.74 0 0 0 5.39-1.61" />
						<path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
						<path d="m2 2 20 20" />
					</svg>
				{:else}
					<!-- eye (lucide) -->
					<svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
						<circle cx="12" cy="12" r="3" />
					</svg>
				{/if}
			</button>
		</div>
	{:else}
		<input
			{id}
			{name}
			{type}
			bind:value
			{placeholder}
			{required}
			{disabled}
			autocomplete={autocomplete as any}
			aria-invalid={error ? 'true' : undefined}
		/>
	{/if}
	{#if error}
		<p class="error" role="alert" style="margin: 0">{error}</p>
	{/if}
</div>
