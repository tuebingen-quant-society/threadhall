import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { ConversationDetail } from "./detail";

describe("ConversationDetail", () => {
	it("provides stable member anchors for message mentions", () => {
		render(<ConversationDetail conversation={null} members={[
			{ user_id: 1, username: "ada", principal_kind: "human", joined_at: "2026-08-25T10:00:00Z" },
		]} />);

		expect(screen.getByText("ada").closest("li")?.id).toBe("member-1");
	});

	it("exposes accessible member pagination", () => {
		const loadMore = vi.fn();
		render(<ConversationDetail conversation={null} members={[]} hasMoreMembers onLoadMoreMembers={loadMore} />);

		fireEvent.click(screen.getByRole("button", { name: "Load more members" }));
		expect(loadMore).toHaveBeenCalledTimes(1);
	});
});
