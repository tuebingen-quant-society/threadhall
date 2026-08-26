import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { ConversationList } from "./list";

const conversations = [
	{ id: 2, kind: "channel" as const, name: "general", created_by: 1, created_at: "2026-08-25T12:00:00Z" },
	{ id: 1, kind: "dm" as const, peer_username: "lin", created_by: 1, created_at: "2026-08-25T11:00:00Z" },
];

describe("ConversationList", () => {
	it("selects a channel through a semantic navigation control", () => {
		const select = vi.fn();
		render(<ConversationList conversations={conversations} selectedId={1} onSelect={select} />);

		fireEvent.click(screen.getByRole("button", { name: "general, public channel" }));
		expect(select).toHaveBeenCalledWith(conversations[0]);
		expect(screen.getByRole("button", { name: "lin" }).getAttribute("aria-current")).toBe("page");
	});

	it("exposes accessible conversation pagination", () => {
		const loadMore = vi.fn();
		render(<ConversationList conversations={conversations} onSelect={vi.fn()} hasMore onLoadMore={loadMore} />);

		fireEvent.click(screen.getByRole("button", { name: "Load more conversations" }));
		expect(loadMore).toHaveBeenCalledTimes(1);
	});

	it("renders active conversation threads as indented child navigation", () => {
		const selectThread = vi.fn();
		render(<ConversationList conversations={conversations} selectedId={2} selectedThreadId={7}
			threads={[{ root: { id: 7, conversation_id: 2, author_id: 1, body: "Review the signal model", rendered_body: "<p>Review the signal model</p>", created_at: "2026-08-26T10:00:00Z" }, reply_count: 3 }]}
			onSelect={vi.fn()} onSelectThread={selectThread} />);

		const child = screen.getByRole("button", { name: "Thread: Review the signal model, 3 replies" });
		expect(child.classList.contains("thread-link")).toBe(true);
		expect(child.getAttribute("aria-current")).toBe("page");
		fireEvent.click(child);
		expect(selectThread).toHaveBeenCalledWith(7);
	});
});
