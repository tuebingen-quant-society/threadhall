export interface ListTicket {
	controller: AbortController;
	epoch: number;
}

export function staleRequest(detail = "request became stale") {
	return new DOMException(detail, "AbortError");
}

export function isAbortError(error: unknown) {
	return error instanceof DOMException && error.name === "AbortError";
}

export class ListCoordinator {
	private refreshTail: Promise<void> = Promise.resolve();
	private refreshController: AbortController | null = null;
	private pagination: ListTicket | null = null;
	private refreshEpoch = 0;
	private refreshing = false;
	private disposed = false;

	private async run<T>(operation: (ticket: ListTicket) => Promise<T>) {
		if (this.disposed) throw staleRequest("list coordinator disposed");
		this.refreshing = true;
		this.refreshEpoch += 1;
		this.pagination?.controller.abort();
		const ticket = { controller: new AbortController(), epoch: this.refreshEpoch };
		this.refreshController = ticket.controller;
		try { return await operation(ticket); }
		finally {
			if (this.refreshController === ticket.controller) this.refreshController = null;
			this.refreshing = false;
		}
	}

	refresh<T>(operation: (ticket: ListTicket) => Promise<T>): Promise<T> {
		const run = this.refreshing ? this.refreshTail.then(() => this.run(operation)) : this.run(operation);
		this.refreshTail = run.then(() => undefined, () => undefined);
		return run;
	}

	async beginPagination(): Promise<ListTicket | null> {
		while (true) {
			const refresh = this.refreshTail;
			await refresh;
			if (refresh === this.refreshTail) break;
		}
		if (this.disposed || this.pagination !== null) return null;
		this.pagination = { controller: new AbortController(), epoch: this.refreshEpoch };
		return this.pagination;
	}

	paginationCurrent(ticket: ListTicket) {
		return this.pagination === ticket && ticket.epoch === this.refreshEpoch && !ticket.controller.signal.aborted;
	}

	finishPagination(ticket: ListTicket) {
		if (this.pagination !== ticket) return false;
		this.pagination = null;
		return true;
	}

	dispose() {
		this.disposed = true;
		this.refreshController?.abort();
		this.pagination?.controller.abort();
	}
}
