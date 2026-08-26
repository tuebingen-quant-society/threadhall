export type ConversationKind = "channel" | "private" | "dm";

export interface User {
	id: number;
	username: string;
	admin: boolean;
	created_at: string;
}

export interface DirectoryUser {
	id: number;
	username: string;
}

export interface UserDirectory {
	users: DirectoryUser[];
}

export interface Session {
	user: User;
	expires_at: string;
}

export interface Conversation {
	id: number;
	kind: ConversationKind;
	name?: string;
	peer_username?: string;
	created_by: number;
	created_at: string;
	unread_count?: number;
}

export interface Member {
	user_id: number;
	username: string;
	principal_kind: "human" | "agent";
	joined_at: string;
}

export interface Capability {
	kind: "plugin" | "skill";
	id: string;
	name: string;
	description: string;
}

export interface CapabilityPage {
	capabilities: Capability[];
}

export interface InlineApp {
	server: string;
	tool: string;
	resource_uri: string;
	html: string;
	arguments: unknown;
	result: unknown;
}

export interface QuestionOption {
	label: string;
	description: string;
}

export interface Question {
	id: string;
	header: string;
	question: string;
	is_other: boolean;
	options: QuestionOption[];
}

export interface Message {
	id: number;
	conversation_id: number;
	author_id: number;
	thread_root_id?: number;
	reply_to_message_id?: number;
	body: string;
	rendered_body: string;
	created_at: string;
	edited_at?: string;
	deleted_at?: string;
	inline_apps?: InlineApp[];
	questions?: Question[];
}

export interface ThreadPage {
	root: Message;
	replies: Message[];
	next_after_id?: number;
}

export interface ThreadSummary {
	root: Message;
	reply_count: number;
	unread_count?: number;
}

export interface ThreadList {
	threads: ThreadSummary[];
}

export interface ConversationFork {
	conversation: Conversation;
	source_conversation_id: number;
	source_root_message_id: number;
}

export interface RealtimeEvent {
	seq: number;
	type: string;
	conversation_id: number;
	entity_id: number;
	payload: unknown;
}

export interface MessageResult {
	message: Message;
	event: RealtimeEvent;
}

export interface ConversationPage {
	conversations: Conversation[];
	next_before_id?: number;
}

export interface MemberPage {
	members: Member[];
	next_before_id?: number;
}

export interface MessagePage {
	messages: Message[];
	next_before_id?: number;
}

export interface ProblemShape {
	status: number;
	code: string;
	detail: string;
}

export type ConnectionState = "connecting" | "connected" | "reconnecting" | "resyncing" | "error" | "offline";
