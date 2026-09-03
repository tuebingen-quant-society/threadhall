import { useMemo, useState } from "preact/hooks";

import { ApiClient } from "./api/client";
import { SessionProvider } from "./auth/session";
import { ChatWorkspace } from "./chat-workspace";
import type { PWAState } from "./pwa/register";
import "./styles.css";

interface AppProps {
	pwaState?: PWAState | null;
	activateUpdate?: () => void;
}

function PWAStatus({ state, activateUpdate }: { state: PWAState | null; activateUpdate: () => void }) {
	const [dismissedState, setDismissedState] = useState<PWAState | null>(null);
	if (state?.kind !== "update-available" && state?.kind !== "error") return null;
	if (dismissedState === state) return null;

	const isUpdate = state.kind === "update-available";
	return <aside class="pwa-status" role="status" aria-live="polite">
		<span>{isUpdate ? "A new version of Threadhall is ready." : "Offline support could not be enabled."}</span>
		<div>
			{isUpdate && <button type="button" class="small-button" onClick={activateUpdate}>Update</button>}
			<button type="button" class="text-button" onClick={() => setDismissedState(state)}>Dismiss</button>
		</div>
	</aside>;
}

export function App({ pwaState = null, activateUpdate = () => undefined }: AppProps) {
	const api = useMemo(() => new ApiClient(), []);
	return <>
		<PWAStatus state={pwaState} activateUpdate={activateUpdate} />
		<SessionProvider api={api}><ChatWorkspace api={api} /></SessionProvider>
	</>;
}
