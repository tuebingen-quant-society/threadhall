import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import type { Message, RealtimeEvent } from "../../api/types";
import { applyRealtimeEvent, mergeMessageResult, queuePending, reconcilePending, Timeline } from "./timeline";

const first: Message = {
	id: 2, conversation_id: 1, author_id: 4, body: "**hello**", rendered_body: "<p><strong>hello</strong></p>",
	created_at: "2026-08-25T12:00:00Z",
};

describe("timeline state", () => {
	it("inserts events chronologically and deduplicates sequence and entity", () => {
		const event: RealtimeEvent = {
			seq: 8, type: "message.sent", conversation_id: 1, entity_id: 3,
			payload: { author_id: 5, body: "later", rendered_body: "<p>later</p>", created_at: "2026-08-25T12:01:00Z" },
		};
		const once = applyRealtimeEvent({ messages: [first], entitySeq: new Map() }, event);
		const twice = applyRealtimeEvent(once, event);

		expect(once.messages.map((message) => message.id)).toEqual([2, 3]);
		expect(twice).toBe(once);
	});

	it("keeps HTTP entity sequence separate from the socket replay cursor", () => {
		const edited = { ...first, body: "HTTP edit", rendered_body: "<p>HTTP edit</p>", edited_at: "2026-08-25T12:09:00Z" };
		const afterHTTP = mergeMessageResult({ messages: [], entitySeq: new Map() }, {
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
});

describe("Timeline", () => {
	it("presents only server-rendered Markdown HTML and edit/delete controls", () => {
		const edit = vi.fn();
		const remove = vi.fn();
		render(<Timeline messages={[first]} currentUserId={4} memberNames={new Map([[4, "ada"]])} onEdit={edit} onDelete={remove} />);

		expect(screen.getByText("hello").tagName).toBe("STRONG");
		expect(screen.queryByText("**hello**")).toBeNull();
		fireEvent.click(screen.getByRole("button", { name: "Edit message" }));
		fireEvent.input(screen.getByLabelText("Edit message text"), { target: { value: "changed" } });
		fireEvent.click(screen.getByRole("button", { name: "Save edit" }));
		fireEvent.click(screen.getByRole("button", { name: "Delete message" }));

		expect(edit).toHaveBeenCalledWith(first, "changed");
		expect(remove).toHaveBeenCalledWith(first);
	});
});
