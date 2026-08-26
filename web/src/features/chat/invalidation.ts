export interface InvalidationCoalescer {
	request(): void;
	stop(): void;
}

const MAX_RETRY_DELAY_MS = 15_000;

function retryDelay(attempt: number) {
	return Math.min(MAX_RETRY_DELAY_MS, 500 * 2 ** Math.min(attempt - 1, 5));
}

export function createInvalidationCoalescer(refresh: () => Promise<void>): InvalidationCoalescer {
	let running = false;
	let queued = false;
	let stopped = false;
	let retryAttempt = 0;
	let retryTimer: ReturnType<typeof setTimeout> | null = null;

	function scheduleRetry() {
		if (stopped || retryTimer !== null) return;
		retryAttempt += 1;
		retryTimer = setTimeout(() => {
			retryTimer = null;
			void drain();
		}, retryDelay(retryAttempt));
	}

	async function drain() {
		if (running || stopped || retryTimer !== null) return;
		running = true;
		queued = false;
		try {
			await refresh();
			retryAttempt = 0;
		} catch (error) {
			if (!stopped) {
				queued = true;
				if (!(error instanceof DOMException && error.name === "AbortError")) scheduleRetry();
			}
		}
		running = false;
		if (queued && retryTimer === null && !stopped) void drain();
	}

	return {
		request() { queued = true; void drain(); },
		stop() {
			stopped = true; queued = false;
			if (retryTimer !== null) clearTimeout(retryTimer);
			retryTimer = null;
		},
	};
}
