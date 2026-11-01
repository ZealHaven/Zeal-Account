<script lang="ts">
	let { children } = $props();
</script>

<main>
	{@render children()}
</main>

<style>
	:global(body) {
		margin: 0;
		font-family:
			system-ui,
			-apple-system,
			sans-serif;
		background: #0f1115;
		color: #e6e8ee;
	}

	main {
		max-width: 36rem;
		margin: 0 auto;
		padding: 3rem 1.5rem;
	}
</style>
