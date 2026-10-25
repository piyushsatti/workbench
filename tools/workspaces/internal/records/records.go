package records

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

// Record is one JSON object from a TODO.jsonl or LOGS.jsonl file, decoded
// loosely: every field is kept, and a missing one reads as blank.
type Record struct {
	// Line is 1-based and counts every line of the file, blank ones included.
	Line   int
	Raw    string
	Fields map[string]json.RawMessage
}

// The only errors a BadLine carries, so a report never quotes the file.
var (
	ErrNotJSON   = errors.New("not valid JSON")
	ErrNotObject = errors.New("not a JSON object")
	// ErrNotRegular rejects a FIFO, device or directory, which could block a
	// read or never end.
	ErrNotRegular = errors.New("not a regular file")
)

type BadLine struct {
	Line int
	Err  error
}

// Read returns the file's records in file order, without the "_meta": true
// header record. A line that is not a JSON object is returned as a BadLine
// and the rest of the file is still read. A missing file is an error wrapping
// os.ErrNotExist; a path that is not a regular file is ErrNotRegular.
func Read(path string) ([]Record, []BadLine, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, ErrNotRegular
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var recs []Record
	var bad []BadLine
	br := bufio.NewReader(f)
	for n := 1; ; n++ {
		line, err := br.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, nil, err
		}
		if raw := bytes.TrimSpace(line); len(raw) > 0 {
			var fields map[string]json.RawMessage
			switch {
			case !json.Valid(raw):
				bad = append(bad, BadLine{Line: n, Err: ErrNotJSON})
			case json.Unmarshal(raw, &fields) != nil || fields == nil:
				bad = append(bad, BadLine{Line: n, Err: ErrNotObject})
			default:
				r := Record{Line: n, Raw: string(raw), Fields: fields}
				if r.Text("_meta") != "true" {
					recs = append(recs, r)
				}
			}
		}
		if err != nil {
			return recs, bad, nil
		}
	}
}

// Text renders a field for display: a string as is, null or a missing field
// as "", a list as its rendered elements joined with ",", anything else as
// compact JSON.
func (r Record) Text(key string) string {
	raw, ok := r.Fields[key]
	if !ok {
		return ""
	}
	return render(raw)
}

// First returns the first of keys whose field renders non-blank.
func (r Record) First(keys ...string) string {
	for _, k := range keys {
		if v := r.Text(k); v != "" {
			return v
		}
	}
	return ""
}

func render(raw json.RawMessage) string {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return string(raw)
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, el := range x {
			b, _ := json.Marshal(el)
			if s := render(b); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return string(raw)
		}
		return buf.String()
	}
}
