import { describe, expect, it } from "vitest";

import { showNewMessageNotification, type BrowserNotifications } from "./browser";

class FakeNotification {
	static permission: NotificationPermission = "granted";
	static requested = 0;
	static instances: Array<{ title: string; options?: NotificationOptions }> = [];
	static async requestPermission() { FakeNotification.requested += 1; return FakeNotification.permission; }
	constructor(title: string, options?: NotificationOptions) { FakeNotification.instances.push({ title, options }); }
}

function notifications() {
	FakeNotification.instances = [];
	FakeNotification.permission = "granted";
	return FakeNotification as unknown as BrowserNotifications;
}

describe("showNewMessageNotification", () => {
	it("notifies about another person's message when its conversation is not active", () => {
		showNewMessageNotification({
			notifications: notifications(), currentUserID: 1, authorID: 8, messageID: 44,
			conversationName: "research", authorName: "bea", isConversationActive: false,
		});

		expect(FakeNotification.instances).toEqual([{
			title: "New message in research",
			options: { body: "bea sent you a message", tag: "threadhall-message-44" },
		}]);
	});

	it("does not interrupt the sender or an active conversation", () => {
		const api = notifications();
		showNewMessageNotification({
			notifications: api, currentUserID: 1, authorID: 1, messageID: 44,
			conversationName: "research", authorName: "ada", isConversationActive: false,
		});
		showNewMessageNotification({
			notifications: api, currentUserID: 1, authorID: 8, messageID: 45,
			conversationName: "research", authorName: "bea", isConversationActive: true,
		});

		expect(FakeNotification.instances).toEqual([]);
	});
});
