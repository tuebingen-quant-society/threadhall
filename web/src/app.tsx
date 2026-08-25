import { useMemo } from "preact/hooks";

import { ApiClient } from "./api/client";
import { SessionProvider } from "./auth/session";
import { ChatWorkspace } from "./chat-workspace";
import "./styles.css";

export function App() {
	const api = useMemo(() => new ApiClient(), []);
	return <SessionProvider api={api}><ChatWorkspace api={api} /></SessionProvider>;
}
