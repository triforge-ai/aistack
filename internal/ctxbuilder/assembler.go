package ctxbuilder

import (
	"strings"
)

// Assemble renders a Context into the final prompt string handed to a provider.
func Assemble(c Context) string {
	var b strings.Builder

	if c.System != "" {
		b.WriteString(c.System)
		b.WriteString("\n\n")
	}

	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("[" + title + "]\n")
		b.WriteString(strings.Join(items, "\n\n"))
		b.WriteString("\n\n")
	}

	section("RULES", c.Rules)
	section("SKILLS", c.Skills)

	if len(c.Memory) > 0 {
		mems := make([]string, 0, len(c.Memory))
		for _, m := range c.Memory {
			mems = append(mems, m.Content)
		}
		section("MEMORY", mems)
	}

	b.WriteString("[TASK]\n")
	b.WriteString(c.Task)
	b.WriteString("\n")

	return b.String()
}
