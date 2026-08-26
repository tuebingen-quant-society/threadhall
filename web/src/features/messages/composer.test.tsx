import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { Composer } from "./composer";

const original = { id: 9, conversation_id: 3, author_id: 2, body: "original message", rendered_body: "<p>original message</p>", created_at: "2026-08-26T10:00:00Z" };

describe("Composer", () => {
	it("completes a member mention without sending the draft", () => {
		const send = vi.fn();
		render(<Composer conversationName="general" onSend={send} mentionCandidates={[
			{ user_id: 7, username: "lin", principal_kind: "human", joined_at: "2026-08-25T10:00:00Z" },
			{ user_id: 8, username: "codex", principal_kind: "agent", joined_at: "2026-08-25T10:00:00Z" },
		]} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;

		fireEvent.input(input, { target: { value: "Ask @li" } });
		const option = screen.getByRole("option", { name: /@lin/ });
		expect(option.textContent).toContain("Person");
		fireEvent.keyDown(input, { key: "Enter" });

		expect(input.value).toBe("Ask @lin ");
		expect(send).not.toHaveBeenCalled();
	});

	it("navigates mention results with arrows and closes them with Escape", () => {
		render(<Composer conversationName="general" onSend={vi.fn()} mentionCandidates={[
			{ user_id: 7, username: "lin", principal_kind: "human", joined_at: "2026-08-25T10:00:00Z" },
			{ user_id: 8, username: "lina", principal_kind: "human", joined_at: "2026-08-25T10:00:00Z" },
		]} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;
		fireEvent.input(input, { target: { value: "@li" } });
		fireEvent.keyDown(input, { key: "ArrowDown" });
		fireEvent.keyDown(input, { key: "Enter" });
		expect(input.value).toBe("@lina ");

		fireEvent.input(input, { target: { value: "@li" } });
		fireEvent.keyDown(input, { key: "Escape" });
		expect(screen.queryByRole("listbox")).toBeNull();
	});

	it("expands slash commands into real Codex skill and plugin mentions", () => {
		render(<Composer conversationName="general" onSend={vi.fn()} mentionCandidates={[
			{ user_id: 8, username: "codex", principal_kind: "agent", joined_at: "2026-08-25T10:00:00Z" },
		]} capabilities={[
			{ kind: "plugin", id: "google-drive@openai-curated-remote", name: "Google Drive", description: "Search and edit Drive files" },
			{ kind: "skill", id: "better-layout", name: "Better Layout", description: "Improve interface layout" },
		]} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;

		fireEvent.input(input, { target: { value: "/plugin goo" } });
		fireEvent.keyDown(input, { key: "Enter" });
		expect(input.value).toBe("@codex [@Google Drive](plugin://google-drive@openai-curated-remote) ");

		fireEvent.input(input, { target: { value: "/skill bet" } });
		fireEvent.keyDown(input, { key: "Enter" });
		expect(input.value).toBe("@codex $better-layout ");
	});

	it("offers command categories at the beginning of a draft", () => {
		render(<Composer conversationName="general" onSend={vi.fn()} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;
		fireEvent.input(input, { target: { value: "/" } });

		expect(screen.getByRole("option", { name: /plugin/i })).not.toBeNull();
		expect(screen.getByRole("option", { name: /skill/i })).not.toBeNull();
		expect(screen.getByRole("option", { name: /codex/i })).not.toBeNull();
	});

	it("sends with Enter, keeps Shift+Enter as a newline, and has an accessible label", async () => {
		const send = vi.fn().mockResolvedValue(undefined);
		render(<Composer conversationName="general" onSend={send} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;

		fireEvent.input(input, { target: { value: "first line" } });
		fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
		expect(send).not.toHaveBeenCalled();
		fireEvent.keyDown(input, { key: "Enter" });

		await waitFor(() => expect(send).toHaveBeenCalledWith("first line", expect.stringMatching(/^send-/)));
		expect(input.value).toBe("");
	});

	it("shows and sends a cancellable reply target", async () => {
		const send = vi.fn().mockResolvedValue(undefined);
		const cancel = vi.fn();
		render(<Composer conversationName="general" onSend={send} replyTo={original} replyToAuthor="lin" onCancelReply={cancel} />);
		expect(screen.getByText("Replying to lin")).not.toBeNull();
		fireEvent.click(screen.getByRole("button", { name: "Cancel reply" }));
		expect(cancel).toHaveBeenCalledOnce();

		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;
		fireEvent.input(input, { target: { value: "linked reply" } });
		fireEvent.keyDown(input, { key: "Enter" });
		await waitFor(() => expect(send).toHaveBeenCalledWith("linked reply", expect.stringMatching(/^send-/), 9));
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

	it("retries an uncertain send with the same idempotency key", async () => {
		const send = vi.fn().mockRejectedValueOnce(new Error("delivery uncertain")).mockResolvedValueOnce(undefined);
		render(<Composer conversationName="general" onSend={send} />);
		const input = screen.getByLabelText("Message general") as HTMLTextAreaElement;
		fireEvent.input(input, { target: { value: "retry exactly" } });
		fireEvent.keyDown(input, { key: "Enter" });
		await screen.findByRole("alert");

		const firstKey = send.mock.calls[0][1];
		expect(firstKey).toMatch(/^send-/);
		fireEvent.keyDown(input, { key: "Enter" });
		await waitFor(() => {
			expect(send).toHaveBeenCalledTimes(2);
			expect(input.value).toBe("");
		});

		expect(send.mock.calls[1]).toEqual(["retry exactly", firstKey]);
	});
});
