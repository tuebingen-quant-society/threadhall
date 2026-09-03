import { render } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";

import { App } from "./app";
import { registerPWA, type PWAController, type PWAState } from "./pwa/register";

const root = document.getElementById("app");

if (root === null) {
	throw new Error("Threadhall mount element is missing");
}

function Root() {
	const [pwaState, setPWAState] = useState<PWAState | null>(null);
	const pwaController = useRef<PWAController | null>(null);

	useEffect(() => {
		const controller = registerPWA(setPWAState);
		pwaController.current = controller;
		return () => {
			controller.cleanup();
			pwaController.current = null;
		};
	}, []);

	return <App pwaState={pwaState} activateUpdate={() => pwaController.current?.activateUpdate()} />;
}

render(<Root />, root);
