import { useEffect, useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import type { ConversationKind, DirectoryUser, UserDirectory } from "../../api/types";

export type ConversationDraft =
	| { kind: "channel"; name: string }
	| { kind: "private"; name: string; memberIds: number[] }
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
	const [selectedMembers, setSelectedMembers] = useState<DirectoryUser[]>([]);
	const [users, setUsers] = useState<DirectoryUser[]>([]);
	const [finding, setFinding] = useState(false);
	const [directoryError, setDirectoryError] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		if (!open || kind === "channel" || (kind === "dm" && selectedMembers.length > 0)) return;
		const controller = new AbortController();
		setFinding(true); setDirectoryError("");
		void onFindUsers(memberQuery, controller.signal)
			.then((result) => { if (!controller.signal.aborted) setUsers(result.users); })
			.catch((cause) => { if (!controller.signal.aborted) setDirectoryError(errorDetail(cause)); })
			.finally(() => { if (!controller.signal.aborted) setFinding(false); });
		return () => controller.abort();
	}, [kind, memberQuery, onFindUsers, open, selectedMembers.length]);

	async function submit(event: Event) {
		event.preventDefault();
		setError("");
		setBusy(true);
		try {
			if (kind === "dm") {
				if (!selectedMembers[0]) return;
				await onCreate({ kind, otherUserId: selectedMembers[0].id });
			}
			else if (kind === "private") await onCreate({ kind, name, memberIds: selectedMembers.map((member) => member.id) });
			else await onCreate({ kind, name });
			setName("");
			setMemberQuery(""); setSelectedMembers([]); setUsers([]);
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
					setMemberQuery(""); setSelectedMembers([]); setUsers([]); setDirectoryError("");
				}}>
					<option value="channel">Public channel</option>
					<option value="private">Private channel</option>
					<option value="dm">Direct message</option>
				</select></label>
			</div>
			<p class="conversation-kind-help">{kind === "channel" ? "Visible to everyone in the workspace." : kind === "private" ? "Only you and the members you add can open it." : "A private one-to-one conversation."}</p>
			{kind !== "dm" && <label>Channel name<input value={name} maxLength={80} onInput={(event) => setName(event.currentTarget.value)} required /></label>}
			{kind !== "channel" && <div class="member-picker">
				<label>{kind === "dm" ? "Find a member" : "Add members"}<input role="combobox" aria-expanded={kind === "private" || selectedMembers.length === 0} aria-controls="member-results" aria-autocomplete="list" autocomplete="off"
					value={memberQuery} placeholder="Search by username" onInput={(event) => {
						setMemberQuery(event.currentTarget.value); if (kind === "dm") setSelectedMembers([]);
					}} /></label>
				{selectedMembers.length > 0 && <div class="selected-members" aria-label="Selected members">{selectedMembers.map((member) => <button type="button" key={member.id} onClick={() => setSelectedMembers((items) => items.filter((item) => item.id !== member.id))}>{member.username}<span aria-hidden="true"> ×</span></button>)}</div>}
				{(kind === "private" || selectedMembers.length === 0) && <div id="member-results" class="member-results" role="listbox" aria-label="Members">
					{users.filter((user) => !selectedMembers.some((selected) => selected.id === user.id)).map((user) => <button type="button" role="option" aria-selected="false" key={user.id} onClick={() => {
						setSelectedMembers((items) => kind === "dm" ? [user] : [...items, user]); setMemberQuery(kind === "dm" ? user.username : ""); if (kind === "dm") setUsers([]);
					}}>{user.username}</button>)}
					{finding && <span class="muted">Searching…</span>}
					{!finding && !directoryError && users.length === 0 && <span class="muted">No members found.</span>}
				</div>}
				{directoryError && <p class="form-error" role="alert">{directoryError}</p>}
			</div>}
			{error && <p class="form-error" role="alert">{error}</p>}
			<div class="form-actions"><button class="text-button" type="button" onClick={() => setOpen(false)}>Cancel</button><button class="small-button" type="submit" disabled={busy || (kind === "dm" && selectedMembers.length === 0)}>{busy ? "Creating…" : "Create"}</button></div>
		</form>
	);
}
