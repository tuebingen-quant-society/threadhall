export interface HistoryRequestToken { generation: number; epoch: number }

export class HistoryRequestGuard {
	private generation = 0;
	private epoch = 0;

	capture(): HistoryRequestToken { return { generation: this.generation, epoch: this.epoch }; }
	current(token: HistoryRequestToken) { return this.generation === token.generation && this.epoch === token.epoch; }
	currentGeneration() { return this.generation; }
	updateGeneration(generation: number) { this.generation = generation; }
	completeRecovery() { this.epoch += 1; }
	reset() { this.generation = 0; this.epoch += 1; }
}

export const invalidatedHistory = () => new DOMException("History generation changed", "InvalidStateError");
