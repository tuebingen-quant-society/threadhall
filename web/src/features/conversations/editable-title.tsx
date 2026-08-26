import { useEffect, useRef, useState } from "preact/hooks";

import { errorDetail } from "../../api/client";
import { EditIcon } from "../messages/message-icons";

export function EditableTitle({ value, canEdit, label, onSave }: { value: string; canEdit: boolean; label: string; onSave: (value: string) => Promise<unknown> }) {
	const [editing, setEditing] = useState(false);
	const [draft, setDraft] = useState(value);
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState("");
	const input = useRef<HTMLInputElement>(null);
	useEffect(() => { if (!editing) setDraft(value); }, [editing, value]);
	useEffect(() => { if (editing) { input.current?.focus(); input.current?.select(); } }, [editing]);
	async function submit(event: Event) {
		event.preventDefault();
		const name = draft.trim();
		if (name === "" || name === value || saving) { setEditing(false); return; }
		setSaving(true); setError("");
		try { await onSave(name); setEditing(false); }
		catch (cause) { setError(errorDetail(cause)); }
		finally { setSaving(false); }
	}
	if (editing) return <form class="title-editor" onSubmit={submit}>
		<label class="sr-only" for="active-title-editor">{label}</label>
		<input ref={input} id="active-title-editor" value={draft} maxLength={80} disabled={saving}
			onInput={(event) => setDraft(event.currentTarget.value)} onKeyDown={(event) => {
				if (event.key === "Escape") setEditing(false);
				if (event.key === "Enter") { event.preventDefault(); event.currentTarget.form?.requestSubmit(); }
			}} />
		{error && <span role="alert">{error}</span>}
	</form>;
	return <div class="editable-title"><h2>{value}</h2>{canEdit && <button type="button" aria-label={`Edit ${label.toLowerCase()}`} title={`Edit ${label.toLowerCase()}`} onClick={() => setEditing(true)}><EditIcon /></button>}</div>;
}
