import { fireEvent, render, screen } from "@testing-library/preact";
import { describe, expect, it } from "vitest";

import type { BrowserNotifications } from "./browser";
import { NotificationPermissionControl } from "./permission";

class FakeNotification {
	static permission: NotificationPermission = "default";
	static requested = 0;
	static async requestPermission() { FakeNotification.requested += 1; FakeNotification.permission = "granted"; return FakeNotification.permission; }
	constructor(_title: string, _options?: NotificationOptions) {}
}

describe("NotificationPermissionControl", () => {
	it("requests browser permission only after the user chooses to enable notifications", async () => {
		FakeNotification.permission = "default";
		FakeNotification.requested = 0;
		render(<NotificationPermissionControl notifications={FakeNotification as unknown as BrowserNotifications} />);

		fireEvent.click(screen.getByRole("button", { name: "Enable notifications" }));

		expect(FakeNotification.requested).toBe(1);
		expect(await screen.findByText("Notifications on")).toBeTruthy();
	});
});
