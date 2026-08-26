import { afterEach, describe, expect, it, vi } from "vitest";

import { createInvalidationCoalescer } from "./invalidation";

function deferred() {
	let resolve!: () => void;
	const promise = new Promise<void>((done) => { resolve = done; });
	return { promise, resolve };
}

async function settle() {
	await Promise.resolve();
	await Promise.resolve();
}

afterEach(() => vi.useRealTimers());

describe("conversation invalidation coalescer", () => {
	it("turns a 10k event burst into one in-flight reload and one queued refresh", async () => {
		const first = deferred();
		const refresh = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValue(undefined);
		const coalescer = createInvalidationCoalescer(refresh);

		for (let index = 0; index < 10_000; index += 1) coalescer.request();
		expect(refresh).toHaveBeenCalledTimes(1);
		first.resolve();
		await first.promise;
		await Promise.resolve();
		await Promise.resolve();

		expect(refresh).toHaveBeenCalledTimes(2);
		coalescer.stop();
	});

	it("retries a failed refresh and resets after success", async () => {
		vi.useFakeTimers();
		const refresh = vi.fn().mockRejectedValueOnce(new Error("unavailable")).mockResolvedValue(undefined);
		const coalescer = createInvalidationCoalescer(refresh);

		coalescer.request();
		await settle();
		expect(refresh).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(499);
		expect(refresh).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(1);
		expect(refresh).toHaveBeenCalledTimes(2);
		expect(vi.getTimerCount()).toBe(0);
		coalescer.stop();
	});

	it("coalesces replay bursts into one capped retry timer", async () => {
		vi.useFakeTimers();
		const refresh = vi.fn().mockRejectedValue(new Error("unavailable"));
		const coalescer = createInvalidationCoalescer(refresh);

		coalescer.request();
		await settle();
		for (let index = 0; index < 10_000; index += 1) coalescer.request();
		expect(refresh).toHaveBeenCalledTimes(1);
		expect(vi.getTimerCount()).toBe(1);
		for (const delay of [500, 1_000, 2_000, 4_000, 8_000, 15_000]) {
			await vi.advanceTimersByTimeAsync(delay);
			expect(vi.getTimerCount()).toBe(1);
		}
		const attempts = refresh.mock.calls.length;
		await vi.advanceTimersByTimeAsync(14_999);
		expect(refresh).toHaveBeenCalledTimes(attempts);
		await vi.advanceTimersByTimeAsync(1);
		expect(refresh).toHaveBeenCalledTimes(attempts + 1);
		coalescer.stop();
	});

	it("clears a pending retry when stopped", async () => {
		vi.useFakeTimers();
		const refresh = vi.fn().mockRejectedValue(new Error("unavailable"));
		const coalescer = createInvalidationCoalescer(refresh);

		coalescer.request();
		await settle();
		expect(vi.getTimerCount()).toBe(1);
		coalescer.stop();
		expect(vi.getTimerCount()).toBe(0);
		await vi.runAllTimersAsync();
		expect(refresh).toHaveBeenCalledTimes(1);
	});
});
