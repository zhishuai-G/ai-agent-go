package transport

import (
	"bufio"
	"io"
	"strings"
)

// ParseSSE aggregates data lines in one Server-Sent Event and invokes onData
// once per completed event. Provider-specific terminators such as [DONE] and
// message_stop are intentionally left for the provider adapter to interpret.
func ParseSSE(reader io.Reader, onData func([]byte) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var dataLines []string
	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		return onData([]byte(data))
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		if field == "data" {
			dataLines = append(dataLines, value)
		}
	}

	// The SSE specification only dispatches an event after its blank-line
	// delimiter, so a partial final event at EOF is deliberately discarded.
	return scanner.Err()
}
