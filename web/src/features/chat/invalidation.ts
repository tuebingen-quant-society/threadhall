export interface InvalidationCoalescer {
	request(): void;
	stop(): void;
}

export function createInvalidationCoalescer(refresh: () => Promise<void>): InvalidationCoalescer {
	let running = false;
	let queued = false;
	let stopped = false;

	async function drain() {
		if (running || stopped) {
			if (running) queued = true;
			return;
		}
		running = true;
		do {
			queued = false;
			try { await refresh(); }
			catch (error) {
				if (!stopped && error instanceof DOMException && error.name === "AbortError") queued = true;
				// Non-abort refresh failures already own visible error state.
			}
		} while (queued && !stopped);
		running = false;
	}

	return {
		request() { void drain(); },
		stop() { stopped = true; queued = false; },
	};
}
