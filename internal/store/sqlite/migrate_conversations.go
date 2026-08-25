package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxMigratedChannelNameBytes = 80

type migratedChannelName struct {
	id   int64
	name string
}

func renameCollidingV1Channels(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM conversations
		WHERE kind IN ('channel', 'private') ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read v1 channel names: %w", err)
	}
	var channels []migratedChannelName
	for rows.Next() {
		var channel migratedChannelName
		if err := rows.Scan(&channel.id, &channel.name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan v1 channel name: %w", err)
		}
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate v1 channel names: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close v1 channel names: %w", err)
	}

	reserved := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		reserved[sqliteNoCaseKey(channel.name)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		key := sqliteNoCaseKey(channel.name)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			continue
		}
		candidate := uniqueMigratedChannelName(channel, reserved)
		if _, err := tx.ExecContext(ctx, "UPDATE conversations SET name = ? WHERE id = ?", candidate, channel.id); err != nil {
			return fmt.Errorf("rename colliding v1 channel %d: %w", channel.id, err)
		}
		reserved[sqliteNoCaseKey(candidate)] = struct{}{}
	}
	return nil
}

func uniqueMigratedChannelName(channel migratedChannelName, reserved map[string]struct{}) string {
	for disambiguator := int64(1); ; disambiguator++ {
		suffix := "~" + strconv.FormatInt(channel.id, 10)
		if disambiguator > 1 {
			suffix += "~" + strconv.FormatInt(disambiguator, 10)
		}
		candidate := boundedMigratedChannelName(channel.name, suffix)
		if _, exists := reserved[sqliteNoCaseKey(candidate)]; !exists {
			return candidate
		}
	}
}

func boundedMigratedChannelName(name, suffix string) string {
	valid := strings.ToValidUTF8(name, "\uFFFD")
	maxPrefix := maxMigratedChannelNameBytes - len(suffix)
	if maxPrefix < 0 {
		maxPrefix = 0
	}
	if len(valid) > maxPrefix {
		valid = valid[:maxPrefix]
		for !utf8.ValidString(valid) {
			valid = valid[:len(valid)-1]
		}
	}
	return valid + suffix
}

func sqliteNoCaseKey(value string) string {
	bytes := []byte(value)
	for index, char := range bytes {
		if char >= 'A' && char <= 'Z' {
			bytes[index] = char + ('a' - 'A')
		}
	}
	return string(bytes)
}
