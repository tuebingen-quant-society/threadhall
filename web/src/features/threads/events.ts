import type { RealtimeEvent } from "../../api/types";

export function eventThreadRoot(event: RealtimeEvent) {
	if (!event.type.startsWith("message.") || typeof event.payload !== "object" || event.payload === null) return undefined;
	const root = (event.payload as { thread_root_id?: unknown }).thread_root_id;
	return typeof root === "number" && Number.isSafeInteger(root) && root > 0 ? root : undefined;
}
