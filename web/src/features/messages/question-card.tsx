import { useEffect, useState } from "preact/hooks";

import type { Message, Question } from "../../api/types";

export function linkedQuestionAnswer(messages: Message[], currentUserId: number, messageId: number, question: Question) {
	const prefix = `@codex Answer to "${question.question}": `;
	const reply = messages.find((message) => message.author_id === currentUserId && message.reply_to_message_id === messageId && message.body.startsWith(prefix));
	return reply?.body.slice(prefix.length).trim();
}

export function QuestionCard({ question, answered, onAnswer }: { question: Question; answered?: string; onAnswer: (answer: string) => void | Promise<void> }) {
	const [otherOpen, setOtherOpen] = useState(false);
	const [other, setOther] = useState("");
	const [busy, setBusy] = useState(false);
	const [selected, setSelected] = useState(answered);
	useEffect(() => setSelected(answered), [answered]);

	async function answer(value: string) {
		if (busy || value.trim() === "") return;
		setBusy(true);
		try { await onAnswer(value.trim()); setSelected(value.trim()); setOtherOpen(false); } finally { setBusy(false); }
	}

	const customSelected = selected && !question.options.some((option) => option.label === selected);
	return <section class="question-card" aria-label={question.header}>
		<p>{question.question}</p>
		<div class="question-options">
			{question.options.map((option) => <button class={selected === option.label ? "selected" : undefined} type="button" aria-pressed={selected === option.label} disabled={busy || Boolean(selected)} onClick={() => void answer(option.label)}>
				<span>{option.label}</span>{option.description && <small>{option.description}</small>}
			</button>)}
			{question.is_other && !otherOpen && <button class={customSelected ? "selected" : undefined} type="button" aria-pressed={Boolean(customSelected)} disabled={busy || Boolean(selected)} onClick={() => setOtherOpen(true)}><span>{customSelected ? selected : "Other"}</span><small>{customSelected ? "Custom answer" : "Write a different answer."}</small></button>}
		</div>
		{otherOpen && <form class="question-other" onSubmit={(event) => { event.preventDefault(); void answer(other); }}>
			<label class="sr-only" for={`question-${question.id}`}>Other answer</label>
			<input id={`question-${question.id}`} value={other} onInput={(event) => setOther(event.currentTarget.value)} maxLength={512} autoFocus />
			<button class="small-button" type="submit" disabled={busy || other.trim() === ""}>Answer</button>
		</form>}
	</section>;
}
