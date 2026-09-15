package updatersim

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PreMutationError is a positive assertion that application execution made no
// changes. Unknown failures must never be converted to this type from prose.
type PreMutationError struct {
	Code         string
	Cause        error
	RestoreError error
}

func (e *PreMutationError) Error() string {
	if e.RestoreError != nil {
		return fmt.Sprintf("pre-mutation failure (%s): %v; local pointer restoration failed: %v", e.Code, e.Cause, e.RestoreError)
	}
	return fmt.Sprintf("pre-mutation failure (%s): %v", e.Code, e.Cause)
}
func (e *PreMutationError) Unwrap() error { return e.Cause }

// The complete output must be a single canonical root-generated record. The
// root executor must capture child output and never emit this for child errors.
func classifyPreMutationResult(err error, output []byte) *PreMutationError {
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 78 {
		return nil
	}
	actual := strings.TrimSpace(string(output))
	for _, code := range []string{"signature_invalid", "receipt_invalid", "prerequisite_failed"} {
		expected := `MYSOC_APPLY_RESULT_V1:{"phase":"apply","mutation":"none","code":"` + code + `"}`
		if actual == expected {
			return &PreMutationError{Code: code, Cause: err}
		}
	}
	return nil
}

type pointerSnapshot struct {
	current        string
	previous       []byte
	previousExists bool
	previousMode   os.FileMode
}

func (e *FilesystemExecutor) capturePointers(product string) (*pointerSnapshot, error) {
	p := &pointerSnapshot{}
	var err error
	p.current, err = os.Readlink(e.currentLink(product))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("snapshot current: %w", err)
	}
	info, err := os.Lstat(e.previousFile(product))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("previous pointer must be regular")
	}
	p.previousExists = true
	p.previousMode = info.Mode().Perm()
	p.previous, err = os.ReadFile(e.previousFile(product))
	return p, err
}
func (p *pointerSnapshot) restore(e *FilesystemExecutor, product string) error {
	var currentErr error
	if p.current != "" {
		currentErr = e.swapCurrent(product, p.current)
	} else {
		currentErr = os.Remove(e.currentLink(product))
		if os.IsNotExist(currentErr) {
			currentErr = nil
		}
	}
	var previousErr error
	if !p.previousExists {
		previousErr = os.Remove(e.previousFile(product))
		if os.IsNotExist(previousErr) {
			previousErr = nil
		}
	} else {
		// Atomic replacement preserves an earlier predecessor, not just the newly
		// written .previous pointing at the rejected attempt's current release.
		path := e.previousFile(product)
		f, err := os.CreateTemp(filepath.Dir(path), ".restore-previous-*")
		if err != nil {
			previousErr = err
		} else {
			tmp := f.Name()
			defer os.Remove(tmp)
			if _, err = f.Write(p.previous); err == nil {
				err = f.Chmod(p.previousMode)
			}
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(tmp, path)
			}
			previousErr = err
		}
	}
	return errors.Join(currentErr, previousErr)
}
