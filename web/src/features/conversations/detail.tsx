import { useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import type { Conversation, Member } from "../../api/types";
import { conversationLabel } from "./list";
import { UserAvatar } from "../profile/avatar";

export function ConversationDetail({ conversation, members, loading, loadingMoreMembers, error, hasMoreMembers, onLoadMoreMembers, canDelete, onDelete }: {
	conversation: Conversation | null;
	members: Member[];
	loading?: boolean;
	loadingMoreMembers?: boolean;
	error?: string;
	hasMoreMembers?: boolean;
	onLoadMoreMembers?: () => void;
	canDelete?: boolean;
	onDelete?: () => Promise<void>;
}) {
	const [deleting, setDeleting] = useState(false);
	const [deleteError, setDeleteError] = useState("");
	async function deleteChannel() {
		if (!conversation || !onDelete || !confirm(`Delete #${conversationLabel(conversation)} and all of its messages? This cannot be undone.`)) return;
		setDeleting(true); setDeleteError("");
		try { await onDelete(); } catch (cause) { setDeleteError(errorDetail(cause)); } finally { setDeleting(false); }
	}
	return (
		<div class="detail-pane-inner">
			{conversation ? <>
				<h2>{conversationLabel(conversation)}</h2>
				<p class="detail-kind">{conversation.kind === "dm" ? "Direct message" : conversation.kind === "private" ? "Private channel" : "Public channel"}</p>
				<dl><div><dt>Created</dt><dd>{new Date(conversation.created_at).toLocaleDateString()}</dd></div><div><dt>Conversation ID</dt><dd>{conversation.id}</dd></div></dl>
				{canDelete && conversation.kind !== "dm" && <button class="danger-button" type="button" disabled={deleting} onClick={() => void deleteChannel()}>{deleting ? "Deleting…" : "Delete channel"}</button>}
				{deleteError && <p class="inline-error" role="alert">{deleteError}</p>}
			</> : <h2>No conversation selected</h2>}
			<div class="members-heading"><h3>Members</h3><span>{members.length}</span></div>
			{loading && <p class="muted">Loading details…</p>}
			{error && <p class="inline-error" role="alert">{error}</p>}
			<ul class="member-list">{members.map((member) => <li id={`member-${member.user_id}`} key={member.user_id}><UserAvatar userId={member.user_id} username={member.username} /><span><strong>{member.username}</strong><small>{member.principal_kind === "agent" ? "Agent teammate" : `User ${member.user_id}`}</small></span></li>)}</ul>
			{hasMoreMembers && <button class="load-more-members" type="button" disabled={loadingMoreMembers} onClick={onLoadMoreMembers}>{loadingMoreMembers ? "Loading…" : "Load more members"}</button>}
		</div>
	);
}
