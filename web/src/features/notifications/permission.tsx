import { useState } from "preact/hooks";

import { browserNotifications, type BrowserNotifications } from "./browser";

export function NotificationPermissionControl({ notifications = browserNotifications() }: { notifications?: BrowserNotifications }) {
	const [permission, setPermission] = useState<NotificationPermission | undefined>(notifications?.permission);
	const [error, setError] = useState("");
	if (notifications === undefined) return null;
	if (permission === "granted") return <span class="notification-status">Notifications on</span>;
	if (permission === "denied") return <span class="notification-status" title="Allow notifications in your browser settings to turn them on.">Notifications blocked</span>;
	return <span class="notification-control"><button type="button" onClick={() => {
		setError("");
		void notifications.requestPermission().then(setPermission).catch(() => setError("Could not enable notifications."));
	}}>Enable notifications</button>{error && <span role="alert">{error}</span>}</span>;
}
