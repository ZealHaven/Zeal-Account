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
</script>

<div class="field">
	<label for={id}>{label}</label>
	{#if textarea}
		<textarea
			{id}
			{name}
			bind:value
			{placeholder}
			{required}
			{disabled}
			{rows}
		></textarea>
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
		/>
	{/if}
	{#if error}
		<p class="error" role="alert">{error}</p>
	{/if}
</div>

<style>
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		margin-bottom: 1rem;
	}
	label {
		font-size: 0.9rem;
		color: #c9ccd6;
	}
	input,
	textarea {
		background: #1a1d24;
		color: #e6e8ee;
		border: 1px solid #2a2e38;
		border-radius: 6px;
		padding: 0.6rem 0.75rem;
		font-size: 1rem;
		font-family: inherit;
	}
	input:focus,
	textarea:focus {
		outline: none;
		border-color: #5b7cff;
	}
	textarea {
		resize: vertical;
	}
	.error {
		color: #ff6b6b;
		font-size: 0.85rem;
		margin: 0;
	}
</style>