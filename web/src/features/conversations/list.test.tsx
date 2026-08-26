import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { ConversationList } from "./list";

const conversations = [
	{ id: 2, kind: "channel" as const, name: "general", created_by: 1, created_at: "2026-08-25T12:00:00Z" },
	{ id: 1, kind: "dm" as const, created_by: 1, created_at: "2026-08-25T11:00:00Z" },
];

describe("ConversationList", () => {
	it("selects a channel through a semantic navigation control", () => {
		const select = vi.fn();
		render(<ConversationList conversations={conversations} selectedId={1} onSelect={select} />);

		fireEvent.click(screen.getByRole("button", { name: "general, public channel" }));
		expect(select).toHaveBeenCalledWith(conversations[0]);
		expect(screen.getByRole("button", { name: "Direct message 1" }).getAttribute("aria-current")).toBe("page");
	});

	it("exposes accessible conversation pagination", () => {
		const loadMore = vi.fn();
		render(<ConversationList conversations={conversations} onSelect={vi.fn()} hasMore onLoadMore={loadMore} />);

		fireEvent.click(screen.getByRole("button", { name: "Load more conversations" }));
		expect(loadMore).toHaveBeenCalledTimes(1);
	});
});
