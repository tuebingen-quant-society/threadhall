import type { ComponentChildren } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";

interface WorkspaceShellProps {
	navigation: ComponentChildren;
	main: ComponentChildren;
	context: ComponentChildren;
	selectionKey?: number;
}

export function WorkspaceShell({ navigation, main, context, selectionKey }: WorkspaceShellProps) {
	const [navigationOpen, setNavigationOpen] = useState(false);
	const [contextOpen, setContextOpen] = useState(false);
	const navigationClose = useRef<HTMLButtonElement>(null);
	const contextClose = useRef<HTMLButtonElement>(null);

	useEffect(() => setNavigationOpen(false), [selectionKey]);
	useEffect(() => {
		if (navigationOpen) navigationClose.current?.focus();
	}, [navigationOpen]);
	useEffect(() => {
		if (contextOpen) contextClose.current?.focus();
	}, [contextOpen]);

	function openNavigation() {
		setContextOpen(false);
		setNavigationOpen(true);
	}

	function openContext() {
		setNavigationOpen(false);
		setContextOpen(true);
	}

	return (
		<div class="workspace-shell">
			<div class="mobile-toolbar">
				<button type="button" aria-label="Open conversations" aria-expanded={navigationOpen} onClick={openNavigation}>Conversations</button>
				<span>Threadhall</span>
				<button type="button" aria-label="Open conversation details" aria-expanded={contextOpen} onClick={openContext}>Details</button>
			</div>
			<aside class={navigationOpen ? "navigation-pane is-open" : "navigation-pane"} aria-label="Conversation navigation">
				<button ref={navigationClose} class="drawer-close" type="button" aria-label="Close conversations" onClick={() => setNavigationOpen(false)}>Close</button>
				{navigation}
			</aside>
			<main class="conversation-pane">{main}</main>
			<aside class={contextOpen ? "context-pane is-open" : "context-pane"} aria-label="Conversation details">
				<button ref={contextClose} class="drawer-close" type="button" aria-label="Close conversation details" onClick={() => setContextOpen(false)}>Close</button>
				{context}
			</aside>
			{(navigationOpen || contextOpen) && <button class="drawer-backdrop" type="button" aria-label="Close open drawer" onClick={() => { setNavigationOpen(false); setContextOpen(false); }} />}
		</div>
	);
}
