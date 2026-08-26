package codex

import (
	"context"
	"fmt"
	"io"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

func (c Client) Discover(ctx context.Context) ([]agenttask.Capability, error) {
	command, stdin, stdout, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	items, protocolErr := discoverProtocol(ctx, struct { io.Reader; io.Writer }{Reader: stdout, Writer: stdin}, c.Cwd)
	stopProcess(command, stdin)
	if protocolErr != nil {
		return nil, fmt.Errorf("Codex catalog protocol: %w", protocolErr)
	}
	return items, nil
}
