import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import type { ApiClient } from "../../api/client";
import { ProfilePanel } from "./profile";

describe("ProfilePanel", () => {
	it("uploads and removes a bounded profile picture", async () => {
		const api = { setAvatar: vi.fn().mockResolvedValue(undefined), deleteAvatar: vi.fn().mockResolvedValue(undefined) } as unknown as ApiClient;
		render(<ProfilePanel api={api} user={{ id: 4, username: "ada", admin: false, created_at: "2026-08-25T10:00:00Z" }} onClose={vi.fn()} />);
		const file = new File([new Uint8Array([1, 2, 3])], "ada.png", { type: "image/png" });

		fireEvent.change(screen.getByLabelText("Profile picture"), { target: { files: [file] } });
		await waitFor(() => expect(api.setAvatar).toHaveBeenCalledWith(file));
		fireEvent.click(screen.getByRole("button", { name: "Remove picture" }));
		await waitFor(() => expect(api.deleteAvatar).toHaveBeenCalledTimes(1));
	});
});
