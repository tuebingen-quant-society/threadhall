import { useState } from "preact/hooks";

import { errorDetail } from "../../api/client";

interface ComposerProps {
	conversationName: string;
	onSend: (body: string) => Promise<void>;
}

export function Composer({ conversationName, onSend }: ComposerProps) {
	const [draft, setDraft] = useState("");
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	async function send() {
		const body = draft;
		if (busy || body.trim() === "") return;
		setBusy(true);
		setError("");
		try {
			await onSend(body);
			setDraft((current) => current === body ? "" : current);
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
					onInput={(event) => setDraft(event.currentTarget.value)}
					onKeyDown={keyDown}
					placeholder={`Message ${conversationName}`}
					maxLength={16_384}
					rows={2}
					disabled={busy}
				/>
				<button class="send-button" type="button" onClick={() => void send()} disabled={busy || draft.trim() === ""} aria-label="Send message">
					{busy ? "Sending" : "Send"}
				</button>
			</div>
			<p class="composer-hint">Enter to send · Shift+Enter for a new line</p>
		</footer>
	);
}
