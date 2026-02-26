package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type outputFormat string

const (
	formatJSON  outputFormat = "json"
	formatTable outputFormat = "table"
	formatPlain outputFormat = "plain"
)

func parseFormat(s string, def outputFormat) outputFormat {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return def
	}
	switch outputFormat(s) {
	case formatJSON, formatTable, formatPlain:
		return outputFormat(s)
	default:
		return def
	}
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", string(b))
	return err
}
