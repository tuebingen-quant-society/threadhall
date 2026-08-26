import { useEffect, useState } from "preact/hooks";

import { avatarURL } from "../../api/client";

export function UserAvatar({ userId, username, version = 0, className = "member-mark" }: { userId: number; username: string; version?: number; className?: string }) {
	const [failed, setFailed] = useState(false);
	useEffect(() => setFailed(false), [userId, version]);
	return <span class={`${className} user-avatar`} aria-hidden="true">
		<span>{username.slice(0, 1).toUpperCase()}</span>
		{!failed && <img src={avatarURL(userId, version)} alt="" onError={() => setFailed(true)} />}
	</span>;
}
