package comin

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Sample is one line of the Prometheus text exposition format.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// Parse reads the Prometheus text format. It ignores comments, timestamps
// and histogram semantics: it only needs info-style gauges.
func Parse(r io.Reader) ([]Sample, error) {
	var out []Sample
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func parseLine(line string) (Sample, error) {
	s := Sample{Labels: map[string]string{}}
	i := strings.IndexAny(line, "{ ")
	if i <= 0 {
		return s, fmt.Errorf("no value in %q", line)
	}
	s.Name = line[:i]
	rest := line[i:]
	if rest[0] == '{' {
		var err error
		rest, err = parseLabels(rest[1:], s.Labels)
		if err != nil {
			return s, err
		}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return s, fmt.Errorf("no value in %q", line)
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return s, err
	}
	s.Value = v
	return s, nil
}

// parseLabels parses `k="v",...}` and returns what follows the closing brace.
func parseLabels(in string, into map[string]string) (string, error) {
	for {
		in = strings.TrimLeft(in, " ,")
		if strings.HasPrefix(in, "}") {
			return in[1:], nil
		}
		eq := strings.IndexByte(in, '=')
		if eq <= 0 || len(in) < eq+2 || in[eq+1] != '"' {
			return "", fmt.Errorf("malformed labels near %q", in)
		}
		key := strings.TrimSpace(in[:eq])
		in = in[eq+2:]
		var val strings.Builder
		closed := false
		for j := 0; j < len(in); j++ {
			c := in[j]
			if c == '\\' && j+1 < len(in) {
				j++
				switch in[j] {
				case 'n':
					val.WriteByte('\n')
				default:
					val.WriteByte(in[j])
				}
				continue
			}
			if c == '"' {
				in = in[j+1:]
				closed = true
				break
			}
			val.WriteByte(c)
		}
		if !closed {
			return "", fmt.Errorf("unterminated label value for %q", key)
		}
		into[key] = val.String()
	}
}
