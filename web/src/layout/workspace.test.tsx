import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it } from "vitest";

import { WorkspaceShell } from "./workspace";

describe("WorkspaceShell", () => {
	it("tracks mobile navigation and context drawer state accessibly", () => {
		render(<WorkspaceShell navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<span>Members</span>} />);

		fireEvent.click(screen.getByRole("button", { name: "Open conversations" }));
		expect(screen.getByLabelText("Conversation navigation").classList.contains("is-open")).toBe(true);
		fireEvent.click(screen.getByRole("button", { name: "Close conversations" }));
		expect(screen.getByLabelText("Conversation navigation").classList.contains("is-open")).toBe(false);
		fireEvent.click(screen.getByRole("button", { name: "Open conversation details" }));
		expect(screen.getByLabelText("Conversation details").classList.contains("is-open")).toBe(true);
	});
});
