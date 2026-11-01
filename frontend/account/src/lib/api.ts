import { goto } from '$app/navigation';

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
		this.name = 'ApiError';
	}
}

export type ApiOptions = {
	method?: string;
	body?: unknown;
	headers?: Record<string, string>;
	credentials?: RequestCredentials;
};

export async function api<T = void>(path: string, opts: ApiOptions = {}): Promise<T> {
	const { method = 'GET', body, headers, credentials = 'same-origin' } = opts;
	const res = await fetch(path, {
		method,
		credentials,
		headers: {
			...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
			...headers
		},
		body: body !== undefined ? JSON.stringify(body) : undefined
	});

	if (!res.ok) {
		let message = res.statusText || 'Request failed';
		try {
			const data = await res.json();
			if (data && typeof data === 'object') {
				const obj = data as Record<string, unknown>;
				if (typeof obj.error === 'string') message = obj.error;
				else if (typeof obj.message === 'string') message = obj.message;
			}
		} catch {
			// ignore parse errors
		}
		throw new ApiError(res.status, message);
	}

	if (res.status === 204) {
		return undefined as T;
	}
	const text = await res.text();
	if (!text) {
		return undefined as T;
	}
	return JSON.parse(text) as T;
}

export function redirectIfLoggedOut(err: unknown, next?: string): boolean {
	if (err instanceof ApiError && err.status === 401) {
		const url = next ? `/login?next=${encodeURIComponent(next)}` : '/login';
		goto(url);
		return true;
	}
	return false;
}