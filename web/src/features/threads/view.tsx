import { useEffect, useState } from "preact/hooks";

import { ApiClient, errorDetail } from "../../api/client";
import type { ConversationKind, Message, ThreadPage } from "../../api/types";
import { Composer } from "../messages/composer";

interface ThreadViewProps {
	api: ApiClient;
	conversationId: number;
	root: Message;
	memberNames: Map<number, string>;
	revision: number;
	onFork: (messageId: number, kind: Exclude<ConversationKind, "dm">, name: string) => Promise<void>;
}

function ThreadMessage({ message, author }: { message: Message; author: string }) {
	return <article class="thread-message">
		<header><strong>{author}</strong><time dateTime={message.created_at}>{new Date(message.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time></header>
		{message.deleted_at ? <p class="tombstone">Message deleted</p> : <div class="message-body" dangerouslySetInnerHTML={{ __html: message.rendered_body }} />}
	</article>;
}

export function ThreadView({ api, conversationId, root, memberNames, revision, onFork }: ThreadViewProps) {
	const [page, setPage] = useState<ThreadPage>();
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [forkOpen, setForkOpen] = useState(false);
	const [forkName, setForkName] = useState("");
	const [forkKind, setForkKind] = useState<Exclude<ConversationKind, "dm">>("private");
	const [forkBusy, setForkBusy] = useState(false);

	useEffect(() => {
		const controller = new AbortController();
		setLoading(true); setError("");
		void api.thread(conversationId, root.id, controller.signal)
			.then((result) => { if (!controller.signal.aborted) setPage(result); })
			.catch((cause) => { if (!controller.signal.aborted) setError(errorDetail(cause)); })
			.finally(() => { if (!controller.signal.aborted) setLoading(false); });
		return () => controller.abort();
	}, [api, conversationId, revision, root.id]);

	async function send(body: string, key: string) {
		const result = await api.sendThreadReply(conversationId, root.id, body, key);
		setPage((current) => current ? { ...current, replies: [...current.replies.filter((item) => item.id !== result.message.id), result.message] } : current);
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

	return <section class="thread-view" aria-label="Thread conversation">
		<div class="thread-toolbar"><span>{page?.replies.length ?? 0} replies</span><button type="button" onClick={() => setForkOpen((value) => !value)}>Fork to channel</button></div>
		{forkOpen && <form class="thread-fork-form" onSubmit={fork}>
			<label>Name<input value={forkName} maxLength={80} onInput={(event) => setForkName(event.currentTarget.value)} required /></label>
			<label>Visibility<select value={forkKind} onChange={(event) => setForkKind(event.currentTarget.value as Exclude<ConversationKind, "dm">)}><option value="private">Private</option><option value="channel">Public</option></select></label>
			<button class="small-button" type="submit" disabled={forkBusy}>{forkBusy ? "Forking…" : "Create fork"}</button>
		</form>}
		{error && <p class="inline-error thread-error" role="alert">{error}</p>}
		<div class="thread-history" aria-busy={loading}>
			<ThreadMessage message={page?.root ?? root} author={memberNames.get(root.author_id) ?? `User ${root.author_id}`} />
			{page?.replies.map((reply) => <ThreadMessage key={reply.id} message={reply} author={memberNames.get(reply.author_id) ?? `User ${reply.author_id}`} />)}
			{loading && !page && <p class="muted">Loading thread…</p>}
		</div>
		<Composer id={`thread-composer-${root.id}`} conversationName="thread" onSend={send} />
	</section>;
}
