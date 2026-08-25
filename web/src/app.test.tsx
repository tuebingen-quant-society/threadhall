import { render, screen } from "@testing-library/preact";
import { describe, expect, it } from "vitest";

import { App } from "./app";

describe("App", () => {
	it("renders the Threadhall health shell", () => {
		render(<App />);

		expect(screen.getByRole("heading", { name: "Threadhall" })).toBeTruthy();
		expect(screen.getByText("Team chat for humans and agents.")).toBeTruthy();
	});
});
