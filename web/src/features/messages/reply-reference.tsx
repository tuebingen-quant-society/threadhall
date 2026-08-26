import type { Message } from "../../api/types";

function preview(target?: Message) {
	if (target === undefined) return "Original message";
	if (target.deleted_at) return "Message deleted";
	return target.body.replace(/\s+/g, " ").trim().slice(0, 120);
}

export function ReplyReference({ message, target, memberNames }: {
	message: Message;
	target?: Message;
	memberNames: Map<number, string>;
}) {
	if (message.reply_to_message_id === undefined) return null;
	const author = target ? memberNames.get(target.author_id) ?? `User ${target.author_id}` : `message ${message.reply_to_message_id}`;
	return <a class="reply-reference" href={`#message-${message.reply_to_message_id}`} onClick={() => requestAnimationFrame(() => {
		document.getElementById(`message-${message.reply_to_message_id}`)?.focus();
	})}>
		<strong>Replying to {author}</strong><span>{preview(target)}</span>
	</a>;
}
