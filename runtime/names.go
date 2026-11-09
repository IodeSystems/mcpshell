package runtime

import (
	"fmt"
	"strings"
)

// reservedName says what name is when it cannot be a session global: a
// builtin, a command, or a name a loadable toolkit installs. "" when it can.
// A session global under such a name would hide it in every later eval.
func reservedName(commands *CommandRegistry, env *Environment, name string) string {
	switch {
	case env.IsBuiltin(name):
		return "a builtin"
	case commands.Get(name) != nil:
		return "a command"
	}
	if alias, ok := jsCommandAliases[name]; ok && commands.Get(alias) != nil {
		return "a command"
	}
	if l := commands.LoadableOwning(name); l != "" {
		return fmt.Sprintf("a name of the %q toolkit", l)
	}
	return ""
}

// reservedNameError is the message for a session global under a reserved
// name.
func reservedNameError(name, what string) string {
	return fmt.Sprintf("'%s' is %s; pick another name, e.g. %s. A top-level name lasts for the session, so it would hide the %s in every later eval (inside a function or block a name may shadow it)",
		name, what, altName(name), name)
}

// altName suggests a free name for a reserved one: all → myAll.
func altName(name string) string {
	return "my" + strings.ToUpper(name[:1]) + name[1:]
}
