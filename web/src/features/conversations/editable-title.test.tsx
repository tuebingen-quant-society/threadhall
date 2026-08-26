import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it, vi } from "vitest";

import { EditableTitle } from "./editable-title";

describe("EditableTitle", () => {
	it("reveals a semantic edit action and saves with Enter", async () => {
		const save = vi.fn().mockResolvedValue(undefined);
		render(<EditableTitle value="research" canEdit label="Channel name" onSave={save} />);
		fireEvent.click(screen.getByRole("button", { name: "Edit channel name" }));
		const input = screen.getByRole("textbox", { name: "Channel name" });
		fireEvent.input(input, { target: { value: "signals" } });
		fireEvent.keyDown(input, { key: "Enter" });
		expect(save).toHaveBeenCalledWith("signals");
	});

	it("does not expose rename to unauthorized members", () => {
		render(<EditableTitle value="research" canEdit={false} label="Channel name" onSave={vi.fn()} />);
		expect(screen.queryByRole("button", { name: "Edit channel name" })).toBeNull();
	});
});
