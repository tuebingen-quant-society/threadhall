import type { Capability, Member } from "../../api/types";

export interface ComposerSuggestion {
	id: string;
	label: string;
	description: string;
	replacement: string;
	start: number;
	end: number;
}

const commands = [
	{ name: "codex", description: "Ask the agent teammate" },
	{ name: "plugin", description: "Use an installed Agent Plugin" },
	{ name: "skill", description: "Use an installed agent skill" },
];

function agentPrefix(members: Member[]) {
	const agent = members.find((member) => member.principal_kind === "agent");
	return agent ? `@${agent.username} ` : "";
}

function capabilitySuggestions(
	command: "plugin" | "skill",
	query: string,
	draft: string,
	members: Member[],
	capabilities: Capability[],
): ComposerSuggestion[] {
	const prefix = agentPrefix(members);
	return capabilities
		.filter((item) => item.kind === command && `${item.name} ${item.id}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
		.slice(0, 8)
		.map((item) => ({
			id: `${item.kind}:${item.id}`,
			label: item.kind === "plugin" ? `@${item.name}` : `$${item.id}`,
			description: item.description,
			replacement: item.kind === "plugin"
				? `${prefix}[@${item.name}](plugin://${item.id}) `
				: `${prefix}$${item.id} `,
			start: 0,
			end: draft.length,
		}));
}

function slashSuggestions(draft: string, members: Member[], capabilities: Capability[]) {
	const match = draft.match(/^\/([\p{L}\p{N}-]*)(?:\s+(.*))?$/u);
	if (!match) return [];
	const command = match[1].toLocaleLowerCase();
	if ((command === "plugin" || command === "skill") && match[2] !== undefined) {
		return capabilitySuggestions(command, match[2], draft, members, capabilities);
	}
	return commands
		.filter((item) => item.name.startsWith(command))
		.map((item) => ({
			id: `command:${item.name}`,
			label: `/${item.name}`,
			description: item.description,
			replacement: item.name === "codex" ? agentPrefix(members) : `/${item.name} `,
			start: 0,
			end: draft.length,
		}));
}

function mentionSuggestions(draft: string, caret: number, members: Member[]) {
	const before = draft.slice(0, caret);
	const match = before.match(/(?:^|\s)@([\p{L}\p{N}._-]*)$/u);
	if (!match) return [];
	const query = match[1].toLocaleLowerCase();
	const start = caret - match[1].length - 1;
	return members
		.filter((member) => member.username.toLocaleLowerCase().startsWith(query))
		.slice(0, 8)
		.map((member) => ({
			id: `member:${member.user_id}`,
			label: `@${member.username}`,
			description: member.principal_kind === "agent" ? "Agent" : "Person",
			replacement: `@${member.username} `,
			start,
			end: caret,
		}));
}

export function composerSuggestions(
	draft: string,
	caret: number,
	members: Member[],
	capabilities: Capability[],
): ComposerSuggestion[] {
	if (caret === draft.length && draft.startsWith("/")) return slashSuggestions(draft, members, capabilities);
	return mentionSuggestions(draft, caret, members);
}

