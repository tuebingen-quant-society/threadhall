import type { Conversation, Member } from "../../api/types";
import { conversationLabel } from "./list";

export function ConversationDetail({ conversation, members, loading, error }: {
	conversation: Conversation | null;
	members: Member[];
	loading?: boolean;
	error?: string;
}) {
	return (
		<div class="detail-pane-inner">
			<p class="section-kicker">Conversation context</p>
			{conversation ? <>
				<h2>{conversationLabel(conversation)}</h2>
				<p class="detail-kind">{conversation.kind === "dm" ? "Direct message" : conversation.kind === "private" ? "Private channel" : "Public channel"}</p>
				<dl><div><dt>Created</dt><dd>{new Date(conversation.created_at).toLocaleDateString()}</dd></div><div><dt>Conversation ID</dt><dd>{conversation.id}</dd></div></dl>
			</> : <h2>No conversation selected</h2>}
			<div class="members-heading"><h3>Members</h3><span>{members.length}</span></div>
			{loading && <p class="muted">Loading details…</p>}
			{error && <p class="inline-error" role="alert">{error}</p>}
			<ul class="member-list">{members.map((member) => <li key={member.user_id}><span class="member-mark" aria-hidden="true">{member.username.slice(0, 1).toUpperCase()}</span><span><strong>{member.username}</strong><small>User {member.user_id}</small></span></li>)}</ul>
		</div>
	);
}
