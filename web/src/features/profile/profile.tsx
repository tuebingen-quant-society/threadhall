import { useState } from "preact/hooks";

import { errorDetail, type ApiClient } from "../../api/client";
import type { User } from "../../api/types";
import { UserAvatar } from "./avatar";

const allowedTypes = new Set(["image/png", "image/jpeg", "image/webp"]);
const maxAvatarBytes = 256 << 10;

export function ProfilePanel({ api, user, onClose }: { api: ApiClient; user: User; onClose: () => void }) {
	const [version, setVersion] = useState(Date.now());
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	async function choose(event: Event) {
		const file = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.files?.[0] : undefined;
		if (!file) return;
		if (!allowedTypes.has(file.type) || file.size > maxAvatarBytes) {
			setError("Choose a PNG, JPEG, or WebP image up to 256 KB.");
			return;
		}
		setBusy(true); setError("");
		try { await api.setAvatar(file); setVersion(Date.now()); }
		catch (cause) { setError(errorDetail(cause)); }
		finally { setBusy(false); }
	}

	async function remove() {
		setBusy(true); setError("");
		try { await api.deleteAvatar(); setVersion(Date.now()); }
		catch (cause) { setError(errorDetail(cause)); }
		finally { setBusy(false); }
	}

	return <div class="profile-panel">
		<header><h2>Profile</h2><button class="text-button" type="button" onClick={onClose}>Close</button></header>
		<div class="profile-identity">
			<UserAvatar userId={user.id} username={user.username} version={version} className="profile-avatar" />
			<div><strong>{user.username}</strong><small>{user.admin ? "Workspace administrator" : "Member"}</small></div>
		</div>
		<label class="avatar-input">Profile picture<input type="file" accept="image/png,image/jpeg,image/webp" disabled={busy} onChange={(event) => void choose(event)} /></label>
		<p class="muted">PNG, JPEG, or WebP · 256 KB maximum</p>
		<button class="text-button" type="button" disabled={busy} onClick={() => void remove()}>Remove picture</button>
		{error && <p class="inline-error" role="alert">{error}</p>}
	</div>;
}
