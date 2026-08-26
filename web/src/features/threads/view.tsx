import { useEffect, useState } from "preact/hooks";

import { ApiClient, errorDetail } from "../../api/client";
import type { Capability, ConversationKind, Member, Message, Question, ThreadPage } from "../../api/types";
import { Composer } from "../messages/composer";
import { MessageBody } from "../messages/body";
import { McpApp } from "../messages/mcp-app";
import { ReplyReference } from "../messages/reply-reference";
import { linkedQuestionAnswer, QuestionCard } from "../messages/question-card";

interface ThreadViewProps {
	api: ApiClient;
	conversationId: number;
	root: Message;
	currentUserId: number;
	memberNames: Map<number, string>;
	members: Member[];
	capabilities: Capability[];
	revision: number;
	onFork: (messageId: number, kind: Exclude<ConversationKind, "dm">, name: string) => Promise<void>;
}

function ThreadMessage({ message, replyTarget, author, memberNames, allMessages, currentUserId, onReply, onQuestionAnswer }: { message: Message; replyTarget?: Message; author: string; memberNames: Map<number, string>; allMessages: Message[]; currentUserId: number; onReply: (message: Message) => void; onQuestionAnswer: (message: Message, question: Question, answer: string) => Promise<void> }) {
	return <article id={`message-${message.id}`} class="thread-message" tabIndex={-1}>
		<header><strong>{author}</strong><time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time><button type="button" onClick={() => onReply(message)}>Reply</button></header>
		<ReplyReference message={message} target={replyTarget} memberNames={memberNames} />
		{message.deleted_at ? <p class="tombstone">Message deleted</p> : <><MessageBody html={message.rendered_body} memberNames={memberNames} />
			{message.inline_apps?.map((app) => <McpApp key={`${app.server}:${app.resource_uri}`} app={app} />)}
			{message.questions?.map((question) => <QuestionCard key={question.id} question={question} answered={linkedQuestionAnswer(allMessages, currentUserId, message.id, question)} onAnswer={(answer) => onQuestionAnswer(message, question, answer)} />)}</>}
	</article>;
}

export function ThreadView({ api, conversationId, root, currentUserId, memberNames, members, capabilities, revision, onFork }: ThreadViewProps) {
	const [page, setPage] = useState<ThreadPage>();
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [forkOpen, setForkOpen] = useState(false);
	const [forkName, setForkName] = useState("");
	const [forkKind, setForkKind] = useState<Exclude<ConversationKind, "dm">>("private");
	const [forkBusy, setForkBusy] = useState(false);
	const [replyingTo, setReplyingTo] = useState<Message>();

	useEffect(() => {
		const controller = new AbortController();
		setLoading(true); setError("");
		void api.thread(conversationId, root.id, controller.signal)
			.then((result) => { if (!controller.signal.aborted) setPage(result); })
			.catch((cause) => { if (!controller.signal.aborted) setError(errorDetail(cause)); })
			.finally(() => { if (!controller.signal.aborted) setLoading(false); });
		return () => controller.abort();
	}, [api, conversationId, revision, root.id]);

	async function send(body: string, key: string, replyToMessageId?: number) {
		const result = await api.sendThreadReply(conversationId, root.id, body, key, undefined, replyToMessageId);
		setPage((current) => current ? { ...current, replies: [...current.replies.filter((item) => item.id !== result.message.id), result.message] } : current);
	}

	async function answerQuestion(message: Message, question: Question, answer: string) {
		setError("");
		try {
			await send(`@codex Answer to "${question.question}": ${answer}`, `answer-${crypto.randomUUID()}`, message.id);
		} catch (cause) {
			setError(errorDetail(cause));
		}
	}

	async function fork(event: Event) {
		event.preventDefault(); setForkBusy(true); setError("");
		try {
			await onFork(root.id, forkKind, forkName);
			setForkOpen(false); setForkName("");
		} catch (cause) {
			setError(errorDetail(cause));
		} finally {
			setForkBusy(false);
		}
	}

	const allMessages = page ? [page.root, ...page.replies] : [root];
	const byID = new Map(allMessages.map((message) => [message.id, message]));
	return <section class="thread-view" aria-label="Thread conversation">
		<div class="thread-toolbar"><span>{page?.replies.length ?? 0} replies</span><button type="button" onClick={() => setForkOpen((value) => !value)}>Fork to channel</button></div>
		{forkOpen && <form class="thread-fork-form" onSubmit={fork}>
			<label>Name<input value={forkName} maxLength={80} onInput={(event) => setForkName(event.currentTarget.value)} required /></label>
			<label>Visibility<select value={forkKind} onChange={(event) => setForkKind(event.currentTarget.value as Exclude<ConversationKind, "dm">)}><option value="private">Private</option><option value="channel">Public</option></select></label>
			<button class="small-button" type="submit" disabled={forkBusy}>{forkBusy ? "Forking…" : "Create fork"}</button>
		</form>}
		{error && <p class="inline-error thread-error" role="alert">{error}</p>}
		<div class="thread-history" aria-busy={loading}>
			<ThreadMessage message={page?.root ?? root} replyTarget={root.reply_to_message_id ? byID.get(root.reply_to_message_id) : undefined} memberNames={memberNames} author={memberNames.get(root.author_id) ?? `User ${root.author_id}`} allMessages={allMessages} currentUserId={currentUserId} onReply={setReplyingTo} onQuestionAnswer={answerQuestion} />
			{page?.replies.map((reply) => <ThreadMessage key={reply.id} message={reply} replyTarget={reply.reply_to_message_id ? byID.get(reply.reply_to_message_id) : undefined} memberNames={memberNames} author={memberNames.get(reply.author_id) ?? `User ${reply.author_id}`} allMessages={allMessages} currentUserId={currentUserId} onReply={setReplyingTo} onQuestionAnswer={answerQuestion} />)}
			{loading && !page && <p class="muted">Loading thread…</p>}
		</div>
		<Composer id={`thread-composer-${root.id}`} conversationName="thread" onSend={send} mentionCandidates={members} capabilities={capabilities}
			replyTo={replyingTo} replyToAuthor={replyingTo ? memberNames.get(replyingTo.author_id) : undefined} onCancelReply={() => setReplyingTo(undefined)} />
	</section>;
}
