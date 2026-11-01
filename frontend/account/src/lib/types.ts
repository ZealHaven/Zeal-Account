export type Account = {
	id: string;
	username: string;
	created_at: string;
};

export type Persona = {
	id: string;
	name: string;
	identity: string | null;
	description: string | null;
	created_at: string;
	updated_at: string;
};

export type RecoveryCodesResponse = {
	recovery_codes: string[];
};

export type MeResponse = {
	account: Account;
	personas: Persona[];
};

export type LoginResponse = {
	account: Account;
	personas: Persona[];
};

export type PersonasResponse = {
	personas: Persona[];
};

export type RegisterResponse = {
	account: { id: string; username: string };
	recovery_codes: string[];
};

export type Notification = {
	id: string;
	type: 'identity_claim_removed' | 'password_changed' | 'password_reset' | 'recovery_codes_regenerated';
	title: string;
	body: string;
	data: Record<string, unknown>;
	read_at: string | null;
	created_at: string;
};