import { fireEvent, render, screen } from "@testing-library/preact";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { WorkspaceShell } from "./workspace";

function media(matches: boolean) {
	return vi.fn().mockImplementation((query: string) => ({
		matches, media: query, onchange: null,
		addEventListener: vi.fn(), removeEventListener: vi.fn(),
		addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(),
	}));
}

describe("WorkspaceShell mobile drawers", () => {
	beforeEach(() => Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: media(true) }));

	it("makes closed drawers inert and traps focus until Escape restores the opener", () => {
		render(<WorkspaceShell navigation={<><button>First channel</button><button>Last channel</button></>} main={<button>Main action</button>} context={<button>Member action</button>} />);
		const navigation = screen.getByLabelText("Conversation navigation");
		const opener = screen.getByRole("button", { name: "Open conversations" });

		expect(navigation.getAttribute("aria-hidden")).toBe("true");
		expect(navigation.hasAttribute("inert")).toBe(true);
		opener.focus();
		fireEvent.click(opener);
		expect(navigation.getAttribute("aria-hidden")).toBe("false");
		expect(screen.getByRole("button", { name: "Close conversations" })).toBe(document.activeElement);

		screen.getByRole("button", { name: "Last channel" }).focus();
		fireEvent.keyDown(document, { key: "Tab" });
		expect(screen.getByRole("button", { name: "Close conversations" })).toBe(document.activeElement);
		fireEvent.keyDown(document, { key: "Escape" });
		expect(opener).toBe(document.activeElement);
		expect(navigation.hasAttribute("inert")).toBe(true);
	});

	it("closes navigation on selection and moves focus into the main workspace", () => {
		const view = render(<WorkspaceShell navigation={<button>Channel</button>} main={<span>Timeline</span>} context={<span>Members</span>} />);
		fireEvent.click(screen.getByRole("button", { name: "Open conversations" }));
		view.rerender(<WorkspaceShell selectionKey={2} navigation={<button>Channel</button>} main={<span>Timeline</span>} context={<span>Members</span>} />);

		expect(screen.getByLabelText("Conversation workspace")).toBe(document.activeElement);
		expect(screen.getByLabelText("Conversation navigation").getAttribute("aria-hidden")).toBe("true");
	});

	it("traps the context drawer and restores its own opener", () => {
		render(<WorkspaceShell navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<button>Member action</button>} />);
		const opener = screen.getByRole("button", { name: "Open conversation details" });
		opener.focus();
		fireEvent.click(opener);
		expect(screen.getByRole("button", { name: "Close conversation details" })).toBe(document.activeElement);
		fireEvent.keyDown(document, { key: "Escape" });
		expect(opener).toBe(document.activeElement);
	});
});

describe("WorkspaceShell desktop panes", () => {
	it("keeps desktop panes available", () => {
		Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: media(false) });
		render(<WorkspaceShell navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<span>Members</span>} />);

		expect(screen.getByLabelText("Conversation navigation").hasAttribute("inert")).toBe(false);
		expect(screen.getByLabelText("Conversation details").getAttribute("aria-hidden")).not.toBe("true");
	});

	it("collapses the details pane to a narrow restore control", () => {
		Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: media(false) });
		render(<WorkspaceShell navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<span>Members</span>} />);

		fireEvent.click(screen.getByRole("button", { name: "Hide details" }));
		expect(screen.getByLabelText("Conversation details").getAttribute("aria-hidden")).toBe("true");
		fireEvent.click(screen.getByRole("button", { name: "Show details" }));
		expect(screen.getByText("Members")).toBeTruthy();
	});

	it("restores a collapsed pane when an artifact requests it", () => {
		Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: media(false) });
		const view = render(<WorkspaceShell navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<span>Preview</span>} />);
		fireEvent.click(screen.getByRole("button", { name: "Hide details" }));
		view.rerender(<WorkspaceShell contextRequestKey="file-1" navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<span>Preview</span>} />);
		expect(screen.getByLabelText("Conversation details").getAttribute("aria-hidden")).not.toBe("true");
	});

	it("uses one context-level close action for a file preview", () => {
		Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: media(false) });
		const close = vi.fn();
		render(<WorkspaceShell navigation={<span>Channels</span>} main={<span>Timeline</span>} context={<span>Preview</span>} onContextClose={close} />);
		fireEvent.click(screen.getByRole("button", { name: "Close details" }));
		expect(close).toHaveBeenCalledOnce();
		expect(screen.getByLabelText("Conversation details").getAttribute("aria-hidden")).toBe("true");
	});
});
