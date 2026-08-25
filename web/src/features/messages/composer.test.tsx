import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { Composer } from "./composer";

describe("Composer", () => {
	it("sends with Enter, keeps Shift+Enter as a newline, and has an accessible label", async () => {
		const send = vi.fn().mockResolvedValue(undefined);
		render(<Composer conversationName="general" onSend={send} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;

		fireEvent.input(input, { target: { value: "first line" } });
		fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
		expect(send).not.toHaveBeenCalled();
		fireEvent.keyDown(input, { key: "Enter" });

		await waitFor(() => expect(send).toHaveBeenCalledWith("first line"));
		expect(input.value).toBe("");
	});

	it("shows send failures without discarding the draft", async () => {
		const send = vi.fn().mockRejectedValue(new Error("service is temporarily unavailable"));
		render(<Composer conversationName="general" onSend={send} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;
		fireEvent.input(input, { target: { value: "keep me" } });
		fireEvent.keyDown(input, { key: "Enter" });

		await screen.findByRole("alert");
		expect(input.value).toBe("keep me");
	});
});
