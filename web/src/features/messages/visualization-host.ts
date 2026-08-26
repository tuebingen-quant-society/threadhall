const styles = `
:root {
  color-scheme: light dark;
  --background: light-dark(#fff, #181818);
  --foreground: light-dark(#1c1c1c, #f4f4f4);
  --card: color-mix(in srgb, var(--foreground) 5%, var(--background));
  --card-foreground: var(--foreground);
  --popover: var(--background);
  --popover-foreground: var(--foreground);
  --primary: light-dark(#1c1c1c, #f4f4f4);
  --primary-foreground: light-dark(#fff, #181818);
  --secondary: color-mix(in srgb, var(--foreground) 6%, var(--background));
  --secondary-foreground: var(--foreground);
  --muted: color-mix(in srgb, var(--foreground) 9%, transparent);
  --muted-foreground: light-dark(#737373, #a3a3a3);
  --accent: color-mix(in srgb, var(--foreground) 8%, var(--background));
  --accent-foreground: var(--foreground);
  --destructive: light-dark(#9b2e20, #ff8a78);
  --border: color-mix(in srgb, var(--foreground) 13%, transparent);
  --input: color-mix(in srgb, var(--foreground) 22%, transparent);
  --ring: var(--foreground);
  --font-size-base: 14px;
  --viz-series-1: light-dark(#315f8c, #86b7e5);
  --viz-series-2: light-dark(#9b5b28, #e4a26e);
  --viz-series-3: light-dark(#47704f, #87c692);
  --viz-series-4: light-dark(#8a4c70, #d695b9);
  --viz-series-5: light-dark(#65508d, #a998d1);
  --viz-series-6: light-dark(#32736e, #79bbb5);
  font: 400 var(--font-size-base)/1.5 Inter, ui-sans-serif, system-ui, sans-serif;
  color: var(--foreground);
  background: var(--background);
}
* { box-sizing: border-box; }
body { margin: 0; padding: 14px; overflow: hidden; }
#widget { display: flex; width: 100%; flex-direction: column; gap: 12px; }
h1, h2, h3, p { margin: 0; }
h1, h2, h3, strong { font-weight: 600; }
h1 { font-size: 1.35rem; } h2 { font-size: 1.18rem; } h3 { font-size: 1rem; }
.text-small, small { font-size: .78rem; } .text-muted { color: var(--muted-foreground); }
.text-destructive { color: var(--destructive); }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; }
.card { padding: 12px; color: var(--card-foreground); background: var(--card); }
.viz-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 10px; }
.viz-row, .viz-controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.viz-stat { display: grid; gap: 2px; } .viz-stat-value { font-size: 1.2rem; font-weight: 600; }
.viz-badge { display: inline-flex; padding: 2px 7px; color: var(--accent-foreground); background: var(--accent); }
.progress { height: 5px; overflow: hidden; background: var(--muted); }
.progress-bar { height: 100%; background: var(--viz-series-1); }
.btn, .form-control, .form-select { min-height: 32px; border: 1px solid var(--input); color: var(--secondary-foreground); background: var(--secondary); font: inherit; }
.btn { display: inline-flex; align-items: center; justify-content: center; padding: 0 9px; cursor: pointer; }
.btn-primary, .btn[aria-pressed="true"], .btn[aria-selected="true"] { color: var(--primary-foreground); background: var(--primary); }
.btn-ghost { border-color: transparent; background: transparent; }
.btn-block, .viz-tile { width: 100%; }
.form-label { display: grid; gap: 4px; }
.form-control, .form-select { width: 100%; padding: 5px 8px; }
.form-range { width: 100%; accent-color: var(--primary); }
.form-check { display: flex; align-items: center; gap: 6px; }
.form-check-input { accent-color: var(--primary); }
.btn:focus-visible, .form-control:focus-visible, .form-select:focus-visible, .form-range:focus-visible { outline: 2px solid var(--ring); outline-offset: 2px; }
.table { width: 100%; border-collapse: collapse; }
.table th, .table td { padding: 7px 12px 7px 0; border-bottom: 1px solid var(--border); text-align: left; vertical-align: top; }
.table-responsive { width: 100%; overflow-x: auto; }
svg { display: block; max-width: 100%; height: auto; }
code { padding: 1px 4px; background: var(--muted); font-family: ui-monospace, monospace; }
@media (max-width: 420px) { body { padding: 10px; } .viz-grid { grid-template-columns: 1fr; } }
`;

const resizeScript = `<script>
(() => {
  const report = () => parent.postMessage({ type: "threadhall/resize", height: document.documentElement.scrollHeight }, "*");
  new ResizeObserver(report).observe(document.documentElement);
  addEventListener("load", report, { once: true });
  report();
})();
</script>`;

export interface VisualizationMetadata {
	title: string;
	mode: "" | "wide";
}

export function visualizationMetadata(value: unknown): VisualizationMetadata {
	if (typeof value !== "object" || value === null) return { title: "Interactive visualization", mode: "" };
	const metadata = value as { title?: unknown; mode?: unknown };
	return {
		title: typeof metadata.title === "string" && metadata.title.trim() ? metadata.title : "Interactive visualization",
		mode: metadata.mode === "wide" ? "wide" : "",
	};
}

export function visualizationDocument(fragment: string) {
	return `<style>${styles}</style>${fragment}${resizeScript}`;
}
