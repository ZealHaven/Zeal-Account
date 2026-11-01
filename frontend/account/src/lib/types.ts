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