import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { ConversationDetail } from "./detail";

describe("ConversationDetail", () => {
	it("exposes accessible member pagination", () => {
		const loadMore = vi.fn();
		render(<ConversationDetail conversation={null} members={[]} hasMoreMembers onLoadMoreMembers={loadMore} />);

		fireEvent.click(screen.getByRole("button", { name: "Load more members" }));
		expect(loadMore).toHaveBeenCalledTimes(1);
	});
});
