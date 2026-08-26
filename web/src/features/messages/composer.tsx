import { useMemo, useRef, useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import type { Capability, Member, Message } from "../../api/types";
import { SendIcon } from "./message-icons";
import { composerSuggestions } from "./suggestions";

interface ComposerProps {
	id?: string;
	conversationName: string;
	onSend: (body: string, idempotencyKey: string, replyToMessageId?: number) => Promise<void>;
	mentionCandidates?: Member[];
	capabilities?: Capability[];
	replyTo?: Message;
	replyToAuthor?: string;
	onCancelReply?: () => void;
}

export function Composer({ id = "message-composer", conversationName, onSend, mentionCandidates = [], capabilities = [], replyTo, replyToAuthor, onCancelReply }: ComposerProps) {
	const [draft, setDraft] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const [caret, setCaret] = useState(0);
	const [active, setActive] = useState(0);
	const [dismissed, setDismissed] = useState(false);
	const attemptKey = useRef<string>();
	const sending = useRef(false);
	const draftValue = useRef("");
	const input = useRef<HTMLTextAreaElement>(null);
	const suggestions = useMemo(() => dismissed ? [] : composerSuggestions(draft, caret, mentionCandidates, capabilities),
		[dismissed, draft, caret, mentionCandidates, capabilities]);
	const listId = `${id}-suggestions`;

	function changeDraft(value: string, nextCaret: number) {
		attemptKey.current = undefined;
		draftValue.current = value;
		setDraft(value); setCaret(nextCaret); setActive(0); setDismissed(false);
	}

	function choose(index: number) {
		const suggestion = suggestions[index];
		if (!suggestion) return;
		const value = draft.slice(0, suggestion.start) + suggestion.replacement + draft.slice(suggestion.end);
		const nextCaret = suggestion.start + suggestion.replacement.length;
		changeDraft(value, nextCaret);
		requestAnimationFrame(() => { input.current?.focus(); input.current?.setSelectionRange(nextCaret, nextCaret); });
	}

	async function send() {
		const body = draftValue.current;
		if (sending.current || body.trim() === "") return;
		const idempotencyKey = attemptKey.current ?? `send-${crypto.randomUUID()}`;
		attemptKey.current = idempotencyKey;
		sending.current = true; setBusy(true);
		setError("");
		let failure = "";
		try {
			if (replyTo) await onSend(body, idempotencyKey, replyTo.id);
			else await onSend(body, idempotencyKey);
			if (draftValue.current === body) {
				attemptKey.current = undefined;
				draftValue.current = "";
				if (input.current) input.current.value = "";
				setDraft(""); setCaret(0);
			}
			onCancelReply?.();
		} catch (cause) {
			failure = errorDetail(cause);
		} finally {
			sending.current = false; setBusy(false);
		}
		if (failure) setError(failure);
	}

	function keyDown(event: KeyboardEvent) {
		if (suggestions.length > 0 && ["ArrowDown", "ArrowUp", "Enter", "Tab", "Escape"].includes(event.key)) {
			event.preventDefault();
			if (event.key === "ArrowDown") setActive((value) => (value + 1) % suggestions.length);
			else if (event.key === "ArrowUp") setActive((value) => (value - 1 + suggestions.length) % suggestions.length);
			else if (event.key === "Escape") setDismissed(true);
			else choose(active);
			return;
		}
		if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
		event.preventDefault();
		void send();
	}

	return (
		<footer class="composer-wrap">
			{error && <p key="error" class="composer-error" role="alert">{error}</p>}
			{replyTo && <div class="composer-reply-target">
				<div><strong>Replying to {replyToAuthor ?? `User ${replyTo.author_id}`}</strong><span>{replyTo.deleted_at ? "Message deleted" : replyTo.body.replace(/\s+/g, " ").slice(0, 120)}</span></div>
				<button type="button" aria-label="Cancel reply" title="Cancel reply" onClick={onCancelReply}>×</button>
			</div>}
			{suggestions.length > 0 && <div key="suggestions" id={listId} class="composer-suggestions" role="listbox" aria-label="Composer suggestions">
				{suggestions.map((suggestion, index) => <button key={suggestion.id} id={`${listId}-${index}`} type="button" role="option"
					aria-selected={index === active} onMouseDown={(event) => event.preventDefault()} onClick={() => choose(index)}>
					<strong>{suggestion.label}</strong><span>{suggestion.description}</span>
				</button>)}
			</div>}
			<div key="composer" class="composer">
				<label class="sr-only" for={id}>Message {conversationName}</label>
				<textarea
					ref={input}
					id={id}
					value={draft}
					onInput={(event) => changeDraft(event.currentTarget.value, event.currentTarget.selectionStart)}
					onSelect={(event) => setCaret(event.currentTarget.selectionStart)}
					onKeyDown={keyDown}
					aria-autocomplete="list"
					aria-expanded={suggestions.length > 0}
					aria-controls={suggestions.length > 0 ? listId : undefined}
					aria-activedescendant={suggestions.length > 0 ? `${listId}-${active}` : undefined}
					placeholder={`Message ${conversationName}`}
					maxLength={16_384}
					rows={2}
					disabled={busy}
				/>
				<button class="send-button" type="button" onClick={() => void send()} disabled={busy || draft.trim() === ""} aria-label={busy ? "Sending message" : "Send message"} title="Send">
					<SendIcon />
				</button>
			</div>
			<p class="composer-hint">Enter to send · Shift+Enter for a new line</p>
		</footer>
	);
}
