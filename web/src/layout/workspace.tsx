import type { ComponentChildren } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";

interface WorkspaceShellProps {
	navigation: ComponentChildren;
	main: ComponentChildren;
	context: ComponentChildren;
	selectionKey?: number | string;
	contextRequestKey?: number | string;
}

function useMedia(query: string) {
	const get = () => typeof matchMedia === "function" && matchMedia(query).matches;
	const [matches, setMatches] = useState(get);
	useEffect(() => {
		const media = matchMedia(query);
		const change = () => setMatches(media.matches);
		media.addEventListener("change", change);
		change();
		return () => media.removeEventListener("change", change);
	}, [query]);
	return matches;
}

function focusable(panel: HTMLElement) {
	return [...panel.querySelectorAll<HTMLElement>("button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])")];
}

export function WorkspaceShell({ navigation, main, context, selectionKey, contextRequestKey }: WorkspaceShellProps) {
	const compact = useMedia("(max-width: 700px)");
	const contextDrawer = useMedia("(max-width: 980px)");
	const [navigationOpen, setNavigationOpen] = useState(false);
	const [contextOpen, setContextOpen] = useState(false);
	const [contextCollapsed, setContextCollapsed] = useState(false);
	const navigationPanel = useRef<HTMLElement>(null);
	const contextPanel = useRef<HTMLElement>(null);
	const navigationOpener = useRef<HTMLButtonElement>(null);
	const contextOpener = useRef<HTMLButtonElement>(null);
	const mainPanel = useRef<HTMLElement>(null);
	const previousSelection = useRef(selectionKey);
	const previousContextRequest = useRef(contextRequestKey);

	const activePanel = compact && navigationOpen ? navigationPanel.current
		: contextDrawer && contextOpen ? contextPanel.current : null;

	useEffect(() => {
		if (!compact) setNavigationOpen(false);
	}, [compact]);
	useEffect(() => {
		if (!contextDrawer) setContextOpen(false); else setContextCollapsed(false);
	}, [contextDrawer]);
	useEffect(() => {
		if (previousSelection.current !== selectionKey && compact && navigationOpen) {
			setNavigationOpen(false);
			mainPanel.current?.focus();
		}
		previousSelection.current = selectionKey;
	}, [compact, navigationOpen, selectionKey]);
	useEffect(() => {
		if (navigationOpen) focusable(navigationPanel.current!)[0]?.focus();
	}, [navigationOpen]);
	useEffect(() => {
		if (contextOpen) focusable(contextPanel.current!)[0]?.focus();
	}, [contextOpen]);
	useEffect(() => {
		if (contextRequestKey === undefined || previousContextRequest.current === contextRequestKey) return;
		setContextCollapsed(false);
		if (contextDrawer) setContextOpen(true);
		previousContextRequest.current = contextRequestKey;
	}, [contextDrawer, contextRequestKey]);

	useEffect(() => {
		if (activePanel === null) return;
		function keyDown(event: KeyboardEvent) {
			if (event.key === "Escape") {
				event.preventDefault();
				if (navigationOpen) {
					setNavigationOpen(false);
					navigationOpener.current?.focus();
				} else {
					setContextOpen(false);
					contextOpener.current?.focus();
				}
				return;
			}
			if (event.key !== "Tab") return;
			const items = focusable(activePanel!);
			if (items.length === 0) return;
			const first = items[0], last = items[items.length - 1];
			if (event.shiftKey && document.activeElement === first) {
				event.preventDefault(); last.focus();
			} else if (!event.shiftKey && document.activeElement === last) {
				event.preventDefault(); first.focus();
			}
		}
		document.addEventListener("keydown", keyDown);
		return () => document.removeEventListener("keydown", keyDown);
	}, [activePanel, contextOpen, navigationOpen]);

	function closeDrawers(restore: "navigation" | "context" | null) {
		setNavigationOpen(false);
		setContextOpen(false);
		if (restore === "navigation") navigationOpener.current?.focus();
		if (restore === "context") contextOpener.current?.focus();
	}

	const navigationHidden = compact && !navigationOpen;
	const contextHidden = contextDrawer ? !contextOpen : contextCollapsed;
	const drawerOpen = (compact && navigationOpen) || (contextDrawer && contextOpen);

	return (
		<div class={contextCollapsed && !contextDrawer ? "workspace-shell context-collapsed" : "workspace-shell"}>
			<div class="mobile-toolbar">
				<button ref={navigationOpener} type="button" aria-label="Open conversations" aria-expanded={navigationOpen} onClick={() => { setContextOpen(false); setNavigationOpen(true); }}>Conversations</button>
				<span>Threadhall</span>
				<button ref={contextOpener} type="button" aria-label="Open conversation details" aria-expanded={contextOpen} onClick={() => { setNavigationOpen(false); setContextOpen(true); }}>Details</button>
			</div>
			<aside ref={navigationPanel} class={navigationOpen ? "navigation-pane is-open" : "navigation-pane"} aria-label="Conversation navigation" aria-hidden={navigationHidden} inert={navigationHidden || undefined} role={compact ? "dialog" : undefined}>
				<button class="drawer-close" type="button" aria-label="Close conversations" onClick={() => closeDrawers("navigation")}>Close</button>
				{navigation}
			</aside>
			<main ref={mainPanel} class="conversation-pane" aria-label="Conversation workspace" tabIndex={-1} inert={drawerOpen || undefined}>{main}</main>
			<div class="context-slot">
				<aside ref={contextPanel} class={`${contextOpen ? "context-pane is-open" : "context-pane"}${contextCollapsed && !contextDrawer ? " is-collapsed" : ""}`} aria-label="Conversation details" aria-hidden={contextHidden} inert={contextHidden || undefined} role={contextDrawer ? "dialog" : undefined}>
					<button class="drawer-close" type="button" aria-label="Close conversation details" onClick={() => closeDrawers("context")}>Close</button>
					{!contextCollapsed && <><button class="context-collapse" type="button" aria-label="Hide details" onClick={() => setContextCollapsed(true)}>Hide</button>{context}</>}
				</aside>
				{contextCollapsed && !contextDrawer && <button class="context-restore" type="button" aria-label="Show details" onClick={() => setContextCollapsed(false)}>Details</button>}
			</div>
			{drawerOpen && <button class="drawer-backdrop" type="button" aria-label="Close open drawer" onClick={() => closeDrawers(navigationOpen ? "navigation" : "context")} />}
		</div>
	);
}
