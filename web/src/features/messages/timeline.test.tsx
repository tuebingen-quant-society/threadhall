import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import type { Message, RealtimeEvent } from "../../api/types";
import type { TimelineState } from "./timeline";
import { applyRealtimeEvent, mergeMessageResult, queuePending, reconcilePending, Timeline } from "./timeline";

const first: Message = {
	id: 2, conversation_id: 1, author_id: 4, body: "**hello**", rendered_body: "<p><strong>hello</strong></p>",
	created_at: "2026-08-25T12:00:00Z",
};

describe("timeline state", () => {
	it("applies inline MCP Apps from the completing agent event", () => {
		const inline_apps = [{ server: "forms", tool: "ask", resource_uri: "ui://forms/ask", html: "<form></form>", arguments: {}, result: {} }];
		const state = applyRealtimeEvent({ messages: [first], entitySeq: new Map(), entityPatches: new Map(), pinnedIds: new Set(), historyGeneration: 0, window: "latest" }, {
			seq: 8, type: "message.edited", conversation_id: 1, entity_id: first.id,
			payload: { body: "Choose", rendered_body: "<p>Choose</p>", edited_at: "2026-08-25T12:01:00Z", inline_apps },
		});
		expect(state.messages[0].inline_apps).toEqual(inline_apps);
	});
	it("inserts events chronologically and deduplicates sequence and entity", () => {
		const event: RealtimeEvent = {
			seq: 8, type: "message.sent", conversation_id: 1, entity_id: 3,
			payload: { author_id: 5, body: "later", rendered_body: "<p>later</p>", created_at: "2026-08-25T12:01:00Z" },
		};
		const once = applyRealtimeEvent({ messages: [first], entitySeq: new Map(), entityPatches: new Map(), pinnedIds: new Set(), historyGeneration: 0, window: "latest" }, event);
		const twice = applyRealtimeEvent(once, event);

		expect(once.messages.map((message) => message.id)).toEqual([2, 3]);
		expect(twice).toBe(once);
	});

	it("keeps HTTP entity sequence separate from the socket replay cursor", () => {
		const edited = { ...first, body: "HTTP edit", rendered_body: "<p>HTTP edit</p>", edited_at: "2026-08-25T12:09:00Z" };
		const afterHTTP = mergeMessageResult({ messages: [], entitySeq: new Map(), entityPatches: new Map(), pinnedIds: new Set(), historyGeneration: 0, window: "latest" }, {
			message: edited,
			event: { seq: 9, type: "message.edited", conversation_id: 1, entity_id: 2, payload: {} },
		});
		const delayed = applyRealtimeEvent(afterHTTP, {
			seq: 8, type: "message.sent", conversation_id: 1, entity_id: 2,
			payload: { author_id: 4, body: "old", rendered_body: "<p>old</p>", created_at: first.created_at },
		});
		const duplicate = applyRealtimeEvent(delayed, {
			seq: 9, type: "message.edited", conversation_id: 1, entity_id: 2,
			payload: { body: "HTTP edit", rendered_body: "<p>HTTP edit</p>", edited_at: edited.edited_at },
		});

		expect(delayed.messages[0].body).toBe("HTTP edit");
		expect(duplicate).toBe(delayed);
	});

	it("reconciles one idempotent pending send with the exact server result", () => {
		const pending = queuePending(queuePending([], "send-key", "hello"), "send-key", "hello");
		const reconciled = reconcilePending(pending, "send-key");

		expect(pending).toHaveLength(1);
		expect(reconciled).toHaveLength(0);
	});

	it("bounds unloaded entity edits and tombstones", () => {
		let state: TimelineState = { messages: [], entitySeq: new Map(), entityPatches: new Map(), pinnedIds: new Set(), historyGeneration: 0, window: "latest" };
		for (let id = 1; id <= 201; id += 1) {
			state = applyRealtimeEvent(state, {
				seq: id, type: id % 2 === 0 ? "message.edited" : "message.deleted",
				conversation_id: 1, entity_id: id,
				payload: id % 2 === 0
					? { body: `edit ${id}`, rendered_body: `<p>edit ${id}</p>`, edited_at: "2026-08-25T12:01:00Z" }
					: { deleted_at: "2026-08-25T12:01:00Z" },
			});
		}

		expect(state.entityPatches.size).toBe(200);
		expect(state.entityPatches.has(1)).toBe(false);
		expect(state.entityPatches.has(201)).toBe(true);
		expect(state.historyGeneration).toBe(1);
	});
});

describe("Timeline", () => {
	it("renders durable agent questions and reports the selected answer", () => {
		const onQuestionAnswer = vi.fn();
		const message: Message = { ...first, questions: [{
			id: "scope", header: "Scope", question: "Where should I look?", is_other: true,
			options: [{ label: "This channel", description: "Use only this conversation." }],
		}] };
		render(<Timeline messages={[message]} currentUserId={4} memberNames={new Map([[4, "codex"]])}
			onEdit={vi.fn()} onDelete={vi.fn()} onOpenThread={vi.fn()} onQuestionAnswer={onQuestionAnswer} />);

		expect(screen.getByText("Where should I look?")).toBeTruthy();
		expect(screen.queryByText("Scope", { selector: ".question-card > strong" })).toBeNull();
		fireEvent.click(screen.getByRole("button", { name: /This channel/ }));
		expect(onQuestionAnswer).toHaveBeenCalledWith(message, message.questions![0], "This channel");
	});
	it("disables a question after the current user posts its linked answer", () => {
		const question = { id: "scope", header: "Scope", question: "Where?", is_other: false,
			options: [{ label: "Here", description: "This channel." }] };
		const message: Message = { ...first, questions: [question] };
		const answer: Message = { ...first, id: 3, author_id: 9, reply_to_message_id: first.id,
			body: '@codex Answer to "Where?": Here', rendered_body: '<p>answer</p>' };
		render(<Timeline messages={[message, answer]} currentUserId={9} memberNames={new Map([[4, "codex"], [9, "admin"]])}
			onEdit={vi.fn()} onDelete={vi.fn()} onOpenThread={vi.fn()} onQuestionAnswer={vi.fn()} />);

		const selected = screen.getByRole("button", { name: /Here/ }) as HTMLButtonElement;
		expect(selected.disabled).toBe(true);
		expect(selected.classList.contains("selected")).toBe(true);
		expect(screen.queryByText("Answered: Here")).toBeNull();
	});
	it("links reply previews to the original and exposes the reply action", () => {
		const reply = { ...first, id: 3, body: "linked reply", rendered_body: "<p>linked reply</p>", reply_to_message_id: first.id };
		const onReply = vi.fn();
		render(<Timeline messages={[first, reply]} currentUserId={4} memberNames={new Map([[4, "ada"]])}
			onEdit={vi.fn()} onDelete={vi.fn()} onOpenThread={vi.fn()} onReply={onReply} />);

		const link = screen.getByRole("link", { name: /Replying to ada.*hello/ });
		expect(link.getAttribute("href")).toBe("#message-2");
		fireEvent.click(screen.getAllByLabelText("Message actions")[1]);
		fireEvent.click(screen.getAllByRole("button", { name: "Reply to message" })[1]);
		expect(onReply).toHaveBeenCalledWith(reply);
	});
	it("shows generated MCP UI inline with its agent message", () => {
		const message = { ...first, inline_apps: [{ server: "forms", tool: "ask", resource_uri: "ui://forms/ask", html: "<form></form>", arguments: {}, result: {} }] };
		render(<Timeline messages={[message]} currentUserId={4} memberNames={new Map([[4, "codex"]])} onEdit={vi.fn()} onDelete={vi.fn()} onOpenThread={vi.fn()} />);
		expect(screen.getByTitle("Interactive UI from forms")).toBeTruthy();
	});
	it("links known member mentions without rewriting code or unknown names", () => {
		const message = {
			...first,
			body: "Ask @lin, not `@lin` or @outsider.",
			rendered_body: "<p>Ask @lin, not <code>@lin</code> or @outsider.</p>",
		};
		render(<Timeline messages={[message]} currentUserId={4} memberNames={new Map([[4, "ada"], [7, "lin"]])} onEdit={vi.fn()} onDelete={vi.fn()} onOpenThread={vi.fn()} />);

		const mention = screen.getByRole("link", { name: "@lin" });
		expect(mention.getAttribute("href")).toBe("#member-7");
		expect(screen.getByText("@lin", { selector: "code" })).not.toBeNull();
		expect(screen.getByText(/@outsider/)).not.toBeNull();
	});

	it("presents only server-rendered Markdown HTML and edit/delete controls", () => {
		const edit = vi.fn();
		const remove = vi.fn();
		const openThread = vi.fn();
		render(<Timeline messages={[first]} currentUserId={4} memberNames={new Map([[4, "ada"]])} onEdit={edit} onDelete={remove} onOpenThread={openThread} />);

		expect(screen.getByText("hello").tagName).toBe("STRONG");
		expect(screen.queryByText("**hello**")).toBeNull();
		fireEvent.click(screen.getByLabelText("Message actions"));
		fireEvent.click(screen.getByRole("button", { name: "Open thread" }));
		fireEvent.click(screen.getByRole("button", { name: "Edit message" }));
		fireEvent.input(screen.getByLabelText("Edit message text"), { target: { value: "changed" } });
		fireEvent.click(screen.getByRole("button", { name: "Save edit" }));
		fireEvent.click(screen.getByLabelText("Message actions"));
		fireEvent.click(screen.getByRole("button", { name: "Delete message" }));

		expect(edit).toHaveBeenCalledWith(first, "changed");
		expect(remove).toHaveBeenCalledWith(first);
		expect(openThread).toHaveBeenCalledWith(first);
	});

	it("moves focus into editing and restores a predictable destination", async () => {
		const remove = vi.fn().mockResolvedValue(undefined);
		render(<Timeline messages={[first]} currentUserId={4} memberNames={new Map([[4, "ada"]])} onEdit={vi.fn()} onDelete={remove} onOpenThread={vi.fn()} />);

		fireEvent.click(screen.getByLabelText("Message actions"));
		fireEvent.click(screen.getByRole("button", { name: "Edit message" }));
		await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("Edit message text")));
		fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
		await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("Message actions")));

		fireEvent.click(screen.getByLabelText("Message actions"));
		fireEvent.click(screen.getByRole("button", { name: "Delete message" }));
		await waitFor(() => expect(document.activeElement).toBe(document.querySelector("[data-message-id='2']")));
	});
});
