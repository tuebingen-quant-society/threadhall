import { describe, expect, it, vi } from "vitest";

import { createInvalidationCoalescer } from "./invalidation";

function deferred() {
	let resolve!: () => void;
	const promise = new Promise<void>((done) => { resolve = done; });
	return { promise, resolve };
}

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
});
