import adapter from '@sveltejs/adapter-node';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	server: {
		// accountd (:8082) is the front door; it proxies here in development.
		// 5273 — 5173 is used by another app on this box.
		port: 5273,
		strictPort: true,
		host: '127.0.0.1',
		allowedHosts: ['localhost', '127.0.0.1']
	},
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Deployed behind accountd (Go front door).
			adapter: adapter({
				out: 'build'
			})
		})
	]
});
