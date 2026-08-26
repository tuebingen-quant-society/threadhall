import { useEffect, useMemo, useRef } from "preact/hooks";

const username = /@([\p{L}\p{N}][\p{L}\p{N}._-]{0,63})/gu;
const excluded = new Set(["A", "CODE", "PRE", "SCRIPT", "STYLE"]);

export function MessageBody({ html, memberNames }: { html: string; memberNames: Map<number, string> }) {
	const root = useRef<HTMLDivElement>(null);
	const members = useMemo(() => new Map([...memberNames].map(([id, name]) => [name.toLocaleLowerCase(), id])), [memberNames]);
	const memberKey = useMemo(() => [...memberNames].map(([id, name]) => `${id}:${name}`).join("\u0000"), [memberNames]);

	useEffect(() => {
		const container = root.current;
		if (!container || members.size === 0) return;
		const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
		const nodes: Text[] = [];
		while (walker.nextNode()) nodes.push(walker.currentNode as Text);
		for (const node of nodes) {
			if (node.parentElement && excluded.has(node.parentElement.tagName)) continue;
			const text = node.data;
			username.lastIndex = 0;
			let match: RegExpExecArray | null;
			let offset = 0;
			let fragment: DocumentFragment | undefined;
			while ((match = username.exec(text))) {
				const previous = match.index > 0 ? text[match.index - 1] : "";
				if (previous && /[\p{L}\p{N}._-]/u.test(previous)) continue;
				const id = members.get(match[1].toLocaleLowerCase());
				if (id === undefined) continue;
				fragment ??= document.createDocumentFragment();
				fragment.append(text.slice(offset, match.index));
				const link = document.createElement("a");
				link.className = "message-mention"; link.href = `#member-${id}`; link.textContent = match[0];
				fragment.append(link); offset = match.index + match[0].length;
			}
			if (fragment) { fragment.append(text.slice(offset)); node.replaceWith(fragment); }
		}
	}, [html, memberKey, members]);

	return <div ref={root} class="message-body" dangerouslySetInnerHTML={{ __html: html }} />;
}
