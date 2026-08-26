import { describe, expect, it } from "vitest";

import type { Conversation, Member } from "../../api/types";
import { appendConversationPage, appendMemberPage, MAX_CLIENT_CONVERSATIONS, MAX_CLIENT_MEMBERS } from "./collection";

function conversation(id: number): Conversation {
	return { id, kind: "channel", name: `channel-${id}`, created_by: 1, created_at: "2026-08-25T12:00:00Z" };
}

function member(user_id: number): Member {
	return { user_id, username: `member-${user_id}`, principal_kind: "human", joined_at: "2026-08-25T12:00:00Z" };
}

describe("bounded conversation collections", () => {
	it("appends and deduplicates conversation pages while preserving the cursor", () => {
		const first = appendConversationPage([], { conversations: [conversation(4), conversation(3)], next_before_id: 3 });
		const second = appendConversationPage(first.items, { conversations: [conversation(3), conversation(2)], next_before_id: 2 });

		expect(second.items.map((item) => item.id)).toEqual([4, 3, 2]);
		expect(second.nextBeforeId).toBe(2);
	});

	it("caps conversations and members at explicit 500-item boundaries", () => {
		const conversations = appendConversationPage([], {
			conversations: Array.from({ length: MAX_CLIENT_CONVERSATIONS + 20 }, (_, index) => conversation(700 - index)),
			next_before_id: 180,
		});
		const members = appendMemberPage([], {
			members: Array.from({ length: MAX_CLIENT_MEMBERS + 20 }, (_, index) => member(700 - index)),
			next_before_id: 180,
		});

		expect(conversations.items).toHaveLength(500);
		expect(conversations.nextBeforeId).toBeUndefined();
		expect(members.items).toHaveLength(500);
		expect(members.nextBeforeId).toBeUndefined();
	});
});
