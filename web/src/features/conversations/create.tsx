import { useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import type { ConversationKind } from "../../api/types";

export type ConversationDraft =
	| { kind: Exclude<ConversationKind, "dm">; name: string }
	| { kind: "dm"; otherUserId: number };

export function NewConversationForm({ onCreate }: { onCreate: (draft: ConversationDraft) => Promise<void> }) {
	const [open, setOpen] = useState(false);
	const [kind, setKind] = useState<ConversationKind>("channel");
	const [name, setName] = useState("");
	const [otherUserId, setOtherUserId] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	async function submit(event: Event) {
		event.preventDefault();
		setError("");
		setBusy(true);
		try {
			if (kind === "dm") await onCreate({ kind, otherUserId: Number(otherUserId) });
			else await onCreate({ kind, name });
			setName("");
			setOtherUserId("");
			setOpen(false);
		} catch (cause) {
			setError(errorDetail(cause));
		} finally {
			setBusy(false);
		}
	}

	if (!open) return <button class="new-conversation" type="button" onClick={() => setOpen(true)}>+ New conversation</button>;

	return (
		<form class="new-conversation-form" onSubmit={submit}>
			<div class="field-row">
				<label>Type<select value={kind} onChange={(event) => setKind(event.currentTarget.value as ConversationKind)}>
					<option value="channel">Public channel</option>
					<option value="private">Private channel</option>
					<option value="dm">Direct message</option>
				</select></label>
			</div>
			{kind === "dm"
				? <label>Member user ID<input type="number" min="1" step="1" value={otherUserId} onInput={(event) => setOtherUserId(event.currentTarget.value)} required /></label>
				: <label>Channel name<input value={name} maxLength={80} onInput={(event) => setName(event.currentTarget.value)} required /></label>}
			{error && <p class="form-error" role="alert">{error}</p>}
			<div class="form-actions"><button class="text-button" type="button" onClick={() => setOpen(false)}>Cancel</button><button class="small-button" type="submit" disabled={busy}>{busy ? "Creating…" : "Create"}</button></div>
		</form>
	);
}
