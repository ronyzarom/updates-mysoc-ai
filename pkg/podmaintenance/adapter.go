package podmaintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"time"
)

func decode(raw []byte, v any) error {
	if e := uniqueJSON(raw); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

type bounded struct{ bytes.Buffer }

func (b *bounded) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, errors.New("adapter output exceeds limit")
	}
	return b.Buffer.Write(p)
}

// CommandAdapter calls a trusted, locally provisioned adapter. It must supervise
// privileged descendants; cancellation is not interpreted as completed work.
type CommandAdapter struct {
	Command []string
	Timeout time.Duration
}

func (a CommandAdapter) Call(ctx context.Context, action string, r Request) (Response, error) {
	var result Response
	if len(a.Command) == 0 || !filepath.IsAbs(a.Command[0]) {
		return result, errors.New("missing adapter command")
	}
	t := a.Timeout
	if t <= 0 {
		t = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	raw, e := json.Marshal(r)
	if e != nil {
		return result, e
	}
	args := append(append([]string{}, a.Command[1:]...), action)
	cmd := exec.CommandContext(ctx, a.Command[0], args...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdin = bytes.NewReader(raw)
	var out, stderr bounded
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if e = cmd.Run(); e != nil {
		return result, e
	}
	if e = decode(out.Bytes(), &result); e != nil {
		return result, e
	}
	return result, nil
}

// encoding/json otherwise accepts duplicate keys with last-value-wins semantics.
func uniqueJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		v, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch v {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || keys[s] {
					return errors.New("duplicate or invalid JSON key")
				}
				keys[s] = true
				if e = walk(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = walk(); e != nil {
					return e
				}
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		_, e = d.Token()
		return e
	}
	if e := walk(); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
