import { useRef, useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import { SendIcon } from "./message-icons";

interface ComposerProps {
	conversationName: string;
	onSend: (body: string, idempotencyKey: string) => Promise<void>;
}

export function Composer({ conversationName, onSend }: ComposerProps) {
	const [draft, setDraft] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const attemptKey = useRef<string>();

	async function send() {
		const body = draft;
		if (busy || body.trim() === "") return;
		const idempotencyKey = attemptKey.current ?? `send-${crypto.randomUUID()}`;
		attemptKey.current = idempotencyKey;
		setBusy(true);
		setError("");
		try {
			await onSend(body, idempotencyKey);
			setDraft((current) => {
				if (current !== body) return current;
				attemptKey.current = undefined;
				return "";
			});
		} catch (cause) {
			setError(errorDetail(cause));
		} finally {
			setBusy(false);
		}
	}

	function keyDown(event: KeyboardEvent) {
		if (event.key !== "Enter" || event.shiftKey || event.isComposing) return;
		event.preventDefault();
		void send();
	}

	return (
		<footer class="composer-wrap">
			{error && <p class="composer-error" role="alert">{error}</p>}
			<div class="composer">
				<label class="sr-only" for="message-composer">Message {conversationName}</label>
				<textarea
					id="message-composer"
					value={draft}
					onInput={(event) => { attemptKey.current = undefined; setDraft(event.currentTarget.value); }}
					onKeyDown={keyDown}
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
