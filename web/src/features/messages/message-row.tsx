import { useEffect, useRef, useState } from "preact/hooks";

import type { Message, Question } from "../../api/types";
import { MessageBody } from "./body";
import { McpApp } from "./mcp-app";
import { DeleteIcon, EditIcon, MoreIcon, ReplyIcon, ThreadIcon } from "./message-icons";
import { ReplyReference } from "./reply-reference";
import { QuestionCard } from "./question-card";

interface MessageRowProps {
	message: Message;
	replyTarget?: Message;
	own: boolean;
	author: string;
	memberNames: Map<number, string>;
	onEdit: (message: Message, body: string) => void | Promise<void>;
	onDelete: (message: Message) => void | Promise<void>;
	onOpenThread: (message: Message) => void;
	onReply?: (message: Message) => void;
	onQuestionAnswer?: (message: Message, question: Question, answer: string) => void | Promise<void>;
	questionAnswers?: Map<string, string | undefined>;
}

export function MessageRow({ message, replyTarget, own, author, memberNames, onEdit, onDelete, onOpenThread, onReply, onQuestionAnswer, questionAnswers }: MessageRowProps) {
	const [editing, setEditing] = useState(false);
	const [draft, setDraft] = useState(message.body);
	const row = useRef<HTMLElement>(null);
	const actionTrigger = useRef<HTMLElement>(null);
	const editField = useRef<HTMLTextAreaElement>(null);
	const restoreActions = useRef(false);
	const deleted = Boolean(message.deleted_at);
	const time = new Date(message.created_at);
	useEffect(() => {
		if (editing) editField.current?.focus();
		else if (restoreActions.current) {
			restoreActions.current = false;
			actionTrigger.current?.focus();
		}
	}, [editing]);

	function stopEditing() {
		restoreActions.current = true;
		setEditing(false);
	}

	function save(event: Event) {
		event.preventDefault();
		if (draft.trim() === "") return;
		void onEdit(message, draft);
		stopEditing();
	}

	async function remove() {
		await onDelete(message);
		row.current?.focus();
	}

	return <article ref={row} id={`message-${message.id}`} class="message-row" data-message-id={message.id} tabIndex={-1}>
		<header>
			<strong>{author}</strong>
			<time dateTime={message.created_at} title={time.toLocaleString()}>{time.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
			{message.edited_at && !deleted && <span>edited</span>}
		</header>
		<ReplyReference message={message} target={replyTarget} memberNames={memberNames} />
		{deleted ? <p class="tombstone">Message deleted</p> : editing ? <form class="edit-form" onSubmit={save}>
			<label class="sr-only" for={`edit-${message.id}`}>Edit message text</label>
			<textarea ref={editField} id={`edit-${message.id}`} value={draft} onInput={(event) => setDraft(event.currentTarget.value)} maxLength={16_384} />
			<div><button class="text-button" type="button" onClick={stopEditing}>Cancel</button><button class="small-button" type="submit">Save edit</button></div>
		</form> : <><MessageBody html={message.rendered_body} memberNames={memberNames} />
			{message.inline_apps?.map((app) => <McpApp key={`${app.server}:${app.resource_uri}`} app={app} />)}
			{onQuestionAnswer && message.questions?.map((question) => <QuestionCard key={question.id} question={question} answered={questionAnswers?.get(question.id)} showQuestion={!message.body.includes(question.question)} onAnswer={(answer) => onQuestionAnswer(message, question, answer)} />)}</>}
		{!deleted && !editing && <details class="message-actions">
			<summary ref={actionTrigger} aria-label="Message actions" title="Message actions"><MoreIcon /></summary>
			<div>
				{onReply && <button type="button" aria-label="Reply to message" title="Reply" onClick={() => onReply(message)}><ReplyIcon /></button>}
				<button type="button" aria-label="Open thread" title="Thread" onClick={() => onOpenThread(message)}><ThreadIcon /></button>
				{own && <button type="button" aria-label="Edit message" title="Edit" onClick={() => setEditing(true)}><EditIcon /></button>}
				{own && <button type="button" aria-label="Delete message" title="Delete" onClick={() => void remove()}><DeleteIcon /></button>}
			</div>
		</details>}
	</article>;
}
