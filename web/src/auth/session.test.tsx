import { fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { LoginPanel } from "./session";

describe("LoginPanel", () => {
	it("shows a stable login error and permits invite registration", async () => {
		const login = vi.fn().mockRejectedValue(new Error("authentication failed"));
		const register = vi.fn().mockResolvedValue(undefined);
		render(<LoginPanel onLogin={login} onRegister={register} />);

		fireEvent.input(screen.getByLabelText("Username"), { target: { value: "ada" } });
		fireEvent.input(screen.getByLabelText("Password"), { target: { value: "wrong password" } });
		fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form")!);
		await screen.findByRole("alert");
		expect(screen.getByText("authentication failed")).toBeTruthy();

		fireEvent.click(screen.getByRole("button", { name: "Redeem an invite" }));
		fireEvent.input(screen.getByLabelText("Invite token"), { target: { value: "invite-token" } });
		fireEvent.input(screen.getByLabelText("Username"), { target: { value: "lin" } });
		fireEvent.input(screen.getByLabelText("Password"), { target: { value: "correct horse battery staple" } });
		fireEvent.submit(screen.getByRole("button", { name: "Create account" }).closest("form")!);

		await waitFor(() => expect(register).toHaveBeenCalledWith("invite-token", "lin", "correct horse battery staple"));
	});
});
