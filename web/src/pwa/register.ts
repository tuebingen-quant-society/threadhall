export type PWAState =
	| { kind: "unsupported" }
	| { kind: "ready" }
	| { kind: "update-available" }
	| { kind: "error"; error: unknown };

export interface PWAController {
	cleanup(): void;
	activateUpdate(): void;
}

export function registerPWA(onState: (state: PWAState) => void): PWAController {
	if (!("serviceWorker" in navigator)) {
		onState({ kind: "unsupported" });
		return { activateUpdate() {}, cleanup() {} };
	}

	let disposed = false;
	let activationRequested = false;
	let registrationStarted = false;
	let registration: ServiceWorkerRegistration | null = null;
	const listenerCleanups: Array<() => void> = [];
	const emit = (state: PWAState) => {
		if (!disposed) onState(state);
	};
	const listen = (target: EventTarget, event: string, listener: EventListener) => {
		target.addEventListener(event, listener);
		listenerCleanups.push(() => target.removeEventListener(event, listener));
	};

	const watchInstallingWorker = (worker: ServiceWorker) => {
		const onStateChange = () => {
			if (worker.state === "installed" && registration?.waiting === worker) emit({ kind: "update-available" });
		};
		listen(worker, "statechange", onStateChange);
		onStateChange();
	};

	const onUpdateFound = () => {
		if (registration?.installing !== null && registration?.installing !== undefined) {
			watchInstallingWorker(registration.installing);
		}
	};
	const onControllerChange = () => {
		if (!activationRequested) return;
		activationRequested = false;
		window.location.reload();
	};
	const onLoad = () => {
		if (registrationStarted) return;
		registrationStarted = true;
		navigator.serviceWorker.register("/sw.js")
			.then((nextRegistration) => {
				if (disposed) return;
				registration = nextRegistration;
				listen(nextRegistration, "updatefound", onUpdateFound);
				if (nextRegistration.waiting !== null) emit({ kind: "update-available" });
				else emit({ kind: "ready" });
			})
			.catch((error: unknown) => emit({ kind: "error", error }));
	};

	listen(window, "load", onLoad);
	listen(navigator.serviceWorker, "controllerchange", onControllerChange);
	if (document.readyState === "complete") queueMicrotask(onLoad);

	return {
		activateUpdate() {
			const waiting = registration?.waiting;
			if (waiting === null || waiting === undefined) return;
			waiting.postMessage({ type: "SKIP_WAITING" });
			activationRequested = true;
		},
		cleanup() {
			if (disposed) return;
			disposed = true;
			for (const cleanup of listenerCleanups.splice(0)) cleanup();
		},
	};
}
