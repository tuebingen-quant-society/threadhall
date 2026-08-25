import { createContext, type ComponentChildren } from "preact";
import { useContext, useEffect, useState } from "preact/hooks";

import { ApiClient, ApiProblem, errorDetail } from "../api/client";
import type { User } from "../api/types";

interface SessionValue {
	user: User;
	logout: () => Promise<void>;
}

const SessionContext = createContext<SessionValue | null>(null);

export function useSession() {
	const value = useContext(SessionContext);
	if (value === null) throw new Error("SessionProvider is missing");
	return value;
}

interface LoginPanelProps {
	onLogin: (username: string, password: string) => Promise<void>;
	onRegister: (token: string, username: string, password: string) => Promise<void>;
}

export function LoginPanel({ onLogin, onRegister }: LoginPanelProps) {
	const [registering, setRegistering] = useState(false);
	const [username, setUsername] = useState("");
	const [password, setPassword] = useState("");
	const [invite, setInvite] = useState("");
	const [error, setError] = useState("");
	const [busy, setBusy] = useState(false);

	async function submit(event: Event) {
		event.preventDefault();
		setError("");
		setBusy(true);
		try {
			if (registering) await onRegister(invite, username, password);
			else await onLogin(username, password);
		} catch (cause) {
			setError(errorDetail(cause));
		} finally {
			setBusy(false);
		}
	}

	function switchMode(next: boolean) {
		setRegistering(next);
		setError("");
	}

	return (
		<main class="auth-page">
			<section class="auth-intro" aria-labelledby="auth-title">
				<p class="eyebrow">THREADHALL / PRIVATE WORKSPACE</p>
				<h1 id="auth-title">Ideas move better in a quiet room.</h1>
				<p>Focused channels, direct conversation, and durable context for people building together.</p>
			</section>
			<section class="auth-form-wrap" aria-label={registering ? "Invite registration" : "Sign in"}>
				<div class="auth-switch" aria-label="Authentication method">
					<button type="button" aria-label="Use sign in" class={!registering ? "active" : ""} onClick={() => switchMode(false)}>Sign in</button>
					<button type="button" class={registering ? "active" : ""} onClick={() => switchMode(true)}>Redeem an invite</button>
				</div>
				<form onSubmit={submit}>
					<header>
						<p class="section-kicker">{registering ? "New member" : "Welcome back"}</p>
						<h2>{registering ? "Join the workshop" : "Enter Threadhall"}</h2>
					</header>
					{registering && <label>Invite token<input value={invite} onInput={(event) => setInvite(event.currentTarget.value)} autoComplete="one-time-code" required /></label>}
					<label>Username<input value={username} onInput={(event) => setUsername(event.currentTarget.value)} autoComplete="username" maxLength={64} required /></label>
					<label>Password<input type="password" value={password} onInput={(event) => setPassword(event.currentTarget.value)} autoComplete={registering ? "new-password" : "current-password"} minLength={12} maxLength={128} required /></label>
					{error && <p class="form-error" role="alert">{error}</p>}
					<button class="primary-button" type="submit" disabled={busy}>{busy ? "Working…" : registering ? "Create account" : "Sign in"}</button>
				</form>
			</section>
		</main>
	);
}

export function SessionProvider({ api, children }: { api: ApiClient; children: ComponentChildren }) {
	const [user, setUser] = useState<User | null | undefined>(undefined);
	const [bootError, setBootError] = useState("");
	const [retry, setRetry] = useState(0);

	useEffect(() => {
		const controller = new AbortController();
		setBootError("");
		api.getSession(controller.signal).then((session) => setUser(session.user)).catch((error) => {
			if (controller.signal.aborted) return;
			if (error instanceof ApiProblem && error.code === "authentication_required") setUser(null);
			else setBootError(errorDetail(error));
		});
		return () => controller.abort();
	}, [api, retry]);

	if (user === undefined) return (
		<main class="center-state" aria-live="polite">
			<p class="eyebrow">THREADHALL</p>
			{bootError ? <><h1>Threadhall is out of reach.</h1><p role="alert">{bootError}</p><button onClick={() => setRetry((value) => value + 1)}>Try again</button></> : <><h1>Opening the workshop…</h1><span class="loading-line" /></>}
		</main>
	);

	if (user === null) return <LoginPanel
		onLogin={async (username, password) => setUser((await api.login(username, password)).user)}
		onRegister={async (token, username, password) => setUser((await api.register(token, username, password)).user)}
	/>;

	return <SessionContext.Provider value={{
		user,
		logout: async () => { await api.logout(); setUser(null); },
	}}>{children}</SessionContext.Provider>;
}
