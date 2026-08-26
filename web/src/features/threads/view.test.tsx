import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import type { ApiClient } from "../../api/client";
import type { Message } from "../../api/types";
import { ThreadView } from "./view";

const root: Message = { id: 7, conversation_id: 3, author_id: 1, body: "root", rendered_body: "<p>root</p>", created_at: "2026-08-26T10:00:00Z" };
const reply: Message = { id: 8, conversation_id: 3, author_id: 2, thread_root_id: 7, body: "reply", rendered_body: "<p>reply</p>", created_at: "2026-08-26T10:01:00Z" };

describe("ThreadView", () => {
	it("marks an opened thread read and confirms whole-thread deletion", async () => {
		vi.spyOn(window, "confirm").mockReturnValue(true);
		const api = {
			thread: vi.fn().mockResolvedValue({ root, replies: [reply] }),
			markThreadRead: vi.fn().mockResolvedValue(undefined),
			deleteThread: vi.fn().mockResolvedValue(undefined),
		} as unknown as ApiClient;
		const onDeleted = vi.fn();
		render(<ThreadView api={api} conversationId={3} root={root} currentUserId={1} memberNames={new Map([[1, "ada"]])}
			members={[]} capabilities={[]} revision={0} onFork={vi.fn()} canDelete onDeleted={onDeleted} onRead={vi.fn()} />);

		await waitFor(() => expect(api.markThreadRead).toHaveBeenCalledWith(3, 7, expect.any(AbortSignal)));
		fireEvent.click(screen.getByRole("button", { name: "Delete thread" }));
		await waitFor(() => expect(api.deleteThread).toHaveBeenCalledWith(3, 7));
		expect(onDeleted).toHaveBeenCalledTimes(1);
	});
	it("answers an agent question as a linked thread reply", async () => {
		const questionRoot: Message = { ...root, questions: [{
			id: "scope", header: "Scope", question: "Which scope?", is_other: false,
			options: [{ label: "Thread", description: "Use this thread." }],
		}] };
		const api = {
			thread: vi.fn().mockResolvedValue({ root: questionRoot, replies: [] }),
			markThreadRead: vi.fn().mockResolvedValue(undefined),
			sendThreadReply: vi.fn().mockResolvedValue({ message: reply, event: { seq: 4 } }),
		} as unknown as ApiClient;
		render(<ThreadView api={api} conversationId={3} root={questionRoot} currentUserId={2} memberNames={new Map([[1, "codex"]])}
			members={[]} capabilities={[]} revision={0} onFork={vi.fn()} />);

		fireEvent.click(await screen.findByRole("button", { name: /Thread/ }));
		await waitFor(() => expect(api.sendThreadReply).toHaveBeenCalledWith(
			3, 7, '@codex Answer to "Which scope?": Thread', expect.any(String), undefined, 7,
		));
	});
	it("loads replies, sends in the main thread stream, and creates a channel fork", async () => {
		const api = {
			thread: vi.fn().mockResolvedValue({ root, replies: [reply] }),
			markThreadRead: vi.fn().mockResolvedValue(undefined),
			sendThreadReply: vi.fn().mockResolvedValue({ message: { ...reply, id: 9, body: "new reply", rendered_body: "<p>new reply</p>" }, event: { seq: 4 } }),
		} as unknown as ApiClient;
		const onFork = vi.fn().mockResolvedValue(undefined);
		render(<ThreadView api={api} conversationId={3} root={root} currentUserId={1} memberNames={new Map([[1, "ada"], [2, "lin"]])}
			members={[{ user_id: 1, username: "ada", principal_kind: "human", joined_at: root.created_at }]} capabilities={[]} revision={0} onFork={onFork} />);

		await screen.findByText("reply");
		const composer = screen.getByRole("textbox", { name: "Message thread" });
		fireEvent.input(composer, { target: { value: "new reply" } });
		fireEvent.keyDown(composer, { key: "Enter" });
		await screen.findByText("new reply");

		fireEvent.click(screen.getByRole("button", { name: "Fork to channel" }));
		fireEvent.input(screen.getByLabelText("Name"), { target: { value: "follow-up" } });
		fireEvent.click(screen.getByRole("button", { name: "Create fork" }));
		await waitFor(() => expect(onFork).toHaveBeenCalledWith(7, "private", "follow-up"));
	});
});
