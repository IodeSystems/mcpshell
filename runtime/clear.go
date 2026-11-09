package runtime

import (
	"fmt"
	"slices"
	"strings"
)

// startState is what the session held before its first eval: the toolkits
// loaded and the non-builtin globals the host bound. Clear goes back to it.
type startState struct {
	loaded  []string
	globals map[string]Value
}

// markStart records the start state once, before the first session eval.
func (t *Shell) markStart() {
	if t.start == nil {
		t.start = &startState{loaded: t.commands.loadedNames(), globals: t.globals.UserBindings()}
	}
}

// Clear returns the session to its start state: it unloads the toolkits
// loaded since (their closers run), forgets every kept result and drops
// every session global. It returns what it cleared, in one line. Class
// identity (Map, Error) is kept.
func (t *Shell) Clear() string {
	t.markStart()
	var unloaded []string
	for _, name := range t.commands.loadedNames() {
		if !slices.Contains(t.start.loaded, name) {
			_, _ = t.Unload(name) // a close error still unloads
			unloaded = append(unloaded, name)
		}
	}
	loaded := t.commands.loadedNames()
	for _, name := range t.start.loaded {
		if !slices.Contains(loaded, name) {
			_, _ = t.Load(name)
		}
	}
	forgot := len(t.kept)
	for _, name := range t.kept {
		t.globals.Remove(map[string]struct{}{name: {}})
	}
	t.kept = nil
	var dropped []string
	for name := range t.globals.UserBindings() {
		if _, atStart := t.start.globals[name]; !atStart {
			dropped = append(dropped, name)
		}
	}
	slices.Sort(dropped)
	for _, name := range dropped {
		t.globals.Remove(map[string]struct{}{name: {}})
	}
	for name, v := range t.start.globals {
		t.globals.DefineOrSet(name, v)
	}

	var parts []string
	if len(unloaded) > 0 {
		parts = append(parts, "unloaded "+strings.Join(unloaded, ", "))
	}
	if forgot > 0 {
		parts = append(parts, fmt.Sprintf("forgot %d kept result(s)", forgot))
	}
	if len(dropped) > 0 {
		shown := dropped
		more := ""
		if len(shown) > 10 {
			shown, more = shown[:10], fmt.Sprintf(", … %d more", len(dropped)-10)
		}
		parts = append(parts, fmt.Sprintf("dropped %d global(s): %s%s", len(dropped), strings.Join(shown, ", "), more))
	}
	if len(parts) == 0 {
		return "cleared: the session held nothing"
	}
	return "cleared: " + strings.Join(parts, "; ")
}
