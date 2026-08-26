import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { NewConversationForm } from "./create";

describe("NewConversationForm direct messages", () => {
	it("discovers a member by username and submits its internal ID", async () => {
		const onCreate = vi.fn().mockResolvedValue(undefined);
		const onFindUsers = vi.fn().mockResolvedValue({ users: [{ id: 2, username: "lin" }] });
		render(<NewConversationForm onCreate={onCreate} onFindUsers={onFindUsers} />);

		fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
		fireEvent.change(screen.getByLabelText("Type"), { target: { value: "dm" } });
		const search = screen.getByRole("combobox", { name: "Find a member" });
		fireEvent.input(search, { target: { value: "Li" } });

		await waitFor(() => expect(onFindUsers).toHaveBeenLastCalledWith("Li", expect.any(AbortSignal)));
		fireEvent.click(await screen.findByRole("option", { name: "lin" }));
		fireEvent.click(screen.getByRole("button", { name: "Create" }));

		await waitFor(() => expect(onCreate).toHaveBeenCalledWith({ kind: "dm", otherUserId: 2 }));
		expect(screen.queryByLabelText(/user id/i)).toBeNull();
	});

	it("does not submit free text that has not been selected", async () => {
		const onCreate = vi.fn().mockResolvedValue(undefined);
		render(<NewConversationForm onCreate={onCreate} onFindUsers={vi.fn().mockResolvedValue({ users: [] })} />);

		fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
		fireEvent.change(screen.getByLabelText("Type"), { target: { value: "dm" } });
		fireEvent.input(screen.getByRole("combobox", { name: "Find a member" }), { target: { value: "unknown" } });

		expect((screen.getByRole("button", { name: "Create" }) as HTMLButtonElement).disabled).toBe(true);
		expect(onCreate).not.toHaveBeenCalled();
	});

	it("creates a named private channel with explicitly selected members", async () => {
		const onCreate = vi.fn().mockResolvedValue(undefined);
		const onFindUsers = vi.fn().mockResolvedValue({ users: [{ id: 2, username: "lin" }, { id: 3, username: "ada" }] });
		render(<NewConversationForm onCreate={onCreate} onFindUsers={onFindUsers} />);

		fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
		fireEvent.change(screen.getByLabelText("Type"), { target: { value: "private" } });
		fireEvent.input(screen.getByLabelText("Channel name"), { target: { value: "research" } });
		fireEvent.input(screen.getByRole("combobox", { name: "Add members" }), { target: { value: "" } });
		fireEvent.click(await screen.findByRole("option", { name: "lin" }));
		fireEvent.click(screen.getByRole("option", { name: "ada" }));
		fireEvent.click(screen.getByRole("button", { name: "Create" }));

		await waitFor(() => expect(onCreate).toHaveBeenCalledWith({ kind: "private", name: "research", memberIds: [2, 3] }));
	});
});
