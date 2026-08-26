import { useEffect, useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import type { ConversationKind, DirectoryUser, UserDirectory } from "../../api/types";

export type ConversationDraft =
	| { kind: Exclude<ConversationKind, "dm">; name: string }
	| { kind: "dm"; otherUserId: number };

interface NewConversationFormProps {
	onCreate: (draft: ConversationDraft) => Promise<void>;
	onFindUsers: (query: string, signal: AbortSignal) => Promise<UserDirectory>;
}

export function NewConversationForm({ onCreate, onFindUsers }: NewConversationFormProps) {
	const [open, setOpen] = useState(false);
	const [kind, setKind] = useState<ConversationKind>("channel");
	const [name, setName] = useState("");
	const [memberQuery, setMemberQuery] = useState("");
	const [selectedMember, setSelectedMember] = useState<DirectoryUser>();
	const [users, setUsers] = useState<DirectoryUser[]>([]);
	const [finding, setFinding] = useState(false);
	const [directoryError, setDirectoryError] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		if (!open || kind !== "dm" || selectedMember) return;
		const controller = new AbortController();
		setFinding(true); setDirectoryError("");
		void onFindUsers(memberQuery, controller.signal)
			.then((result) => { if (!controller.signal.aborted) setUsers(result.users); })
			.catch((cause) => { if (!controller.signal.aborted) setDirectoryError(errorDetail(cause)); })
			.finally(() => { if (!controller.signal.aborted) setFinding(false); });
		return () => controller.abort();
	}, [kind, memberQuery, onFindUsers, open, selectedMember]);

	async function submit(event: Event) {
		event.preventDefault();
		setError("");
		setBusy(true);
		try {
			if (kind === "dm") {
				if (!selectedMember) return;
				await onCreate({ kind, otherUserId: selectedMember.id });
			}
			else await onCreate({ kind, name });
			setName("");
			setMemberQuery(""); setSelectedMember(undefined); setUsers([]);
			setOpen(false);
		} catch (cause) {
			setError(errorDetail(cause));
		} finally {
			setBusy(false);
		}
	}

	if (!open) return <button class="new-conversation" type="button" onClick={() => setOpen(true)}><span aria-hidden="true">+</span> New conversation</button>;

	return (
		<form class="new-conversation-form" onSubmit={submit}>
			<div class="field-row">
				<label>Type<select value={kind} onChange={(event) => {
					setKind(event.currentTarget.value as ConversationKind);
					setMemberQuery(""); setSelectedMember(undefined); setUsers([]); setDirectoryError("");
				}}>
					<option value="channel">Public channel</option>
					<option value="private">Private channel</option>
					<option value="dm">Direct message</option>
				</select></label>
			</div>
			{kind === "dm" ? <div class="member-picker">
				<label>Find a member<input role="combobox" aria-expanded={!selectedMember} aria-controls="member-results" aria-autocomplete="list" autocomplete="off"
					value={memberQuery} placeholder="Search by username" onInput={(event) => {
						setMemberQuery(event.currentTarget.value); setSelectedMember(undefined);
					}} /></label>
				{!selectedMember && <div id="member-results" class="member-results" role="listbox" aria-label="Members">
					{users.map((user) => <button type="button" role="option" aria-selected="false" key={user.id} onClick={() => {
						setSelectedMember(user); setMemberQuery(user.username); setUsers([]);
					}}>{user.username}</button>)}
					{finding && <span class="muted">Searching…</span>}
					{!finding && !directoryError && users.length === 0 && <span class="muted">No members found.</span>}
				</div>}
				{directoryError && <p class="form-error" role="alert">{directoryError}</p>}
			</div> : <label>Channel name<input value={name} maxLength={80} onInput={(event) => setName(event.currentTarget.value)} required /></label>}
			{error && <p class="form-error" role="alert">{error}</p>}
			<div class="form-actions"><button class="text-button" type="button" onClick={() => setOpen(false)}>Cancel</button><button class="small-button" type="submit" disabled={busy || (kind === "dm" && !selectedMember)}>{busy ? "Creating…" : "Create"}</button></div>
		</form>
	);
}
