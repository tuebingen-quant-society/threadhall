export type BrowserNotifications = {
	readonly permission: NotificationPermission;
	requestPermission(): Promise<NotificationPermission>;
} & (new (title: string, options?: NotificationOptions) => Notification);

export function browserNotifications(): BrowserNotifications | undefined {
	if (typeof window === "undefined" || !("Notification" in window)) return undefined;
	return window.Notification;
}

export interface NewMessageNotification {
	notifications: BrowserNotifications | undefined;
	currentUserID: number;
	authorID: number;
	messageID: number;
	conversationName: string;
	authorName: string;
	isConversationActive: boolean;
}

export function showNewMessageNotification(input: NewMessageNotification) {
	if (input.notifications?.permission !== "granted" || input.authorID === input.currentUserID || input.isConversationActive) return;
	new input.notifications(`New message in ${input.conversationName}`, {
		body: `${input.authorName} sent you a message`,
		tag: `threadhall-message-${input.messageID}`,
	});
}
