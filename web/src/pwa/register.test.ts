import { afterEach, describe, expect, it, vi } from "vitest";

import { registerPWA, type PWAState } from "./register";

class FakeWorker extends EventTarget {
	state: ServiceWorkerState = "installing";
	postMessage = vi.fn();
}

class FakeRegistration extends EventTarget {
	installing: ServiceWorker | null = null;
	waiting: ServiceWorker | null = null;
}

const originalServiceWorker = Object.getOwnPropertyDescriptor(navigator, "serviceWorker");

afterEach(() => {
	if (originalServiceWorker === undefined) {
		delete (navigator as unknown as { serviceWorker?: ServiceWorkerContainer }).serviceWorker;
	} else {
		Object.defineProperty(navigator, "serviceWorker", originalServiceWorker);
	}
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

function installServiceWorker(register: ReturnType<typeof vi.fn>) {
	Object.defineProperty(navigator, "serviceWorker", {
		configurable: true,
		value: {
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			register,
		},
	});
	return navigator.serviceWorker as unknown as {
		addEventListener: ReturnType<typeof vi.fn>;
		removeEventListener: ReturnType<typeof vi.fn>;
		register: ReturnType<typeof vi.fn>;
	};
}

async function loadPWA() {
	window.dispatchEvent(new Event("load"));
	await Promise.resolve();
	await Promise.resolve();
}

describe("registerPWA", () => {
	it("reports unsupported browsers without installing listeners", () => {
		delete (navigator as unknown as { serviceWorker?: ServiceWorkerContainer }).serviceWorker;
		const states: PWAState[] = [];

		const controller = registerPWA((state) => states.push(state));

		expect(states).toEqual([{ kind: "unsupported" }]);
		expect(controller.activateUpdate()).toBeUndefined();
	});

	it("registers the service worker only after window load", async () => {
		const registration = new FakeRegistration();
		const worker = installServiceWorker(vi.fn().mockResolvedValue(registration));
		const states: PWAState[] = [];

		registerPWA((state) => states.push(state));
		expect(worker.register).not.toHaveBeenCalled();

		await loadPWA();
		expect(worker.register).toHaveBeenCalledWith("/sw.js");
		expect(states).toEqual([{ kind: "ready" }]);
	});

	it("surfaces a waiting worker from the initial registration", async () => {
		const registration = new FakeRegistration();
		registration.waiting = new FakeWorker() as unknown as ServiceWorker;
		installServiceWorker(vi.fn().mockResolvedValue(registration));
		const states: PWAState[] = [];

		registerPWA((state) => states.push(state));
		await loadPWA();

		expect(states).toEqual([{ kind: "update-available" }]);
	});

	it("surfaces a worker once an update finishes installing", async () => {
		const registration = new FakeRegistration();
		const installing = new FakeWorker();
		registration.installing = installing as unknown as ServiceWorker;
		installServiceWorker(vi.fn().mockResolvedValue(registration));
		const states: PWAState[] = [];

		registerPWA((state) => states.push(state));
		await loadPWA();
		registration.dispatchEvent(new Event("updatefound"));
		installing.state = "installed";
		registration.waiting = installing as unknown as ServiceWorker;
		installing.dispatchEvent(new Event("statechange"));

		expect(states).toEqual([{ kind: "ready" }, { kind: "update-available" }]);
	});

	it("does not surface a first-install worker that activates without waiting", async () => {
		const registration = new FakeRegistration();
		const installing = new FakeWorker();
		registration.installing = installing as unknown as ServiceWorker;
		installServiceWorker(vi.fn().mockResolvedValue(registration));
		const states: PWAState[] = [];

		registerPWA((state) => states.push(state));
		await loadPWA();
		registration.dispatchEvent(new Event("updatefound"));
		installing.state = "installed";
		installing.dispatchEvent(new Event("statechange"));

		expect(states).toEqual([{ kind: "ready" }]);
	});

	it("only reloads after explicitly activating a waiting update", async () => {
		const registration = new FakeRegistration();
		const waiting = new FakeWorker();
		registration.waiting = waiting as unknown as ServiceWorker;
		const serviceWorker = installServiceWorker(vi.fn().mockResolvedValue(registration));
		const reload = vi.fn();
		const fakeWindow = Object.assign(new EventTarget(), { location: { reload } }) as unknown as Window;
		vi.stubGlobal("window", fakeWindow);

		const controller = registerPWA(() => undefined);
		await loadPWA();
		const controllerChange = serviceWorker.addEventListener.mock.calls.find(([event]) => event === "controllerchange")?.[1] as EventListener;
		controllerChange(new Event("controllerchange"));
		expect(reload).not.toHaveBeenCalled();

		controller.activateUpdate();
		expect(waiting.postMessage).toHaveBeenCalledWith({ type: "SKIP_WAITING" });
		controllerChange(new Event("controllerchange"));
		expect(reload).toHaveBeenCalledTimes(1);
	});

	it("does not reload after a failed update activation", async () => {
		const registration = new FakeRegistration();
		const waiting = new FakeWorker();
		const failure = new Error("worker is gone");
		waiting.postMessage.mockImplementation(() => { throw failure; });
		registration.waiting = waiting as unknown as ServiceWorker;
		const serviceWorker = installServiceWorker(vi.fn().mockResolvedValue(registration));
		const reload = vi.fn();
		const fakeWindow = Object.assign(new EventTarget(), { location: { reload } }) as unknown as Window;
		vi.stubGlobal("window", fakeWindow);

		const controller = registerPWA(() => undefined);
		await loadPWA();
		const controllerChange = serviceWorker.addEventListener.mock.calls.find(([event]) => event === "controllerchange")?.[1] as EventListener;
		expect(() => controller.activateUpdate()).toThrow(failure);
		controllerChange(new Event("controllerchange"));

		expect(reload).not.toHaveBeenCalled();
	});

	it("reports registration errors without throwing", async () => {
		const failure = new Error("registration denied");
		installServiceWorker(vi.fn().mockRejectedValue(failure));
		const states: PWAState[] = [];

		registerPWA((state) => states.push(state));
		await loadPWA();

		expect(states).toEqual([{ kind: "error", error: failure }]);
	});

	it("removes each listener it installs", async () => {
		const registration = new FakeRegistration();
		const installing = new FakeWorker();
		registration.installing = installing as unknown as ServiceWorker;
		const serviceWorker = installServiceWorker(vi.fn().mockResolvedValue(registration));
		const removeWindowListener = vi.spyOn(window, "removeEventListener");
		const removeRegistrationListener = vi.spyOn(registration, "removeEventListener");
		const removeWorkerListener = vi.spyOn(installing, "removeEventListener");

		const controller = registerPWA(() => undefined);
		await loadPWA();
		registration.dispatchEvent(new Event("updatefound"));
		controller.cleanup();

		expect(removeWindowListener).toHaveBeenCalledWith("load", expect.any(Function));
		expect(serviceWorker.removeEventListener).toHaveBeenCalledWith("controllerchange", expect.any(Function));
		expect(removeRegistrationListener).toHaveBeenCalledWith("updatefound", expect.any(Function));
		expect(removeWorkerListener).toHaveBeenCalledWith("statechange", expect.any(Function));
	});
});
