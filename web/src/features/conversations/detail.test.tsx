import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
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

	it("offers confirmed deletion only when the selected named channel is deletable", async () => {
		vi.spyOn(window, "confirm").mockReturnValue(true);
		const onDelete = vi.fn().mockResolvedValue(undefined);
		const conversation = { id: 7, kind: "private" as const, name: "research", created_by: 1, created_at: "2026-08-25T10:00:00Z" };
		render(<ConversationDetail conversation={conversation} members={[]} canDelete onDelete={onDelete} />);

		fireEvent.click(screen.getByRole("button", { name: "Delete channel" }));
		await waitFor(() => expect(onDelete).toHaveBeenCalledTimes(1));
		expect(window.confirm).toHaveBeenCalledWith("Delete #research and all of its messages? This cannot be undone.");
	});
});
