import { render } from "preact";

import { App } from "./app";

const root = document.getElementById("app");

if (root === null) {
	throw new Error("Threadhall mount element is missing");
}

render(<App />, root);
