package docgen

import (
	"fmt"
	"strings"

	"github.com/andresantos/docsmakercli/internal/langmodel"
)

// BuildReadme assembles the final README.md content from the LLM-generated
// project overview plus the real project structure (packages/types) taken
// directly from the CodeModel — the structure section is never
// hallucinated, only the prose overview is model-generated.
func BuildReadme(projectName, overview string, model *langmodel.CodeModel) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", projectName)
	fmt.Fprintf(&b, "## Overview\n\n%s\n\n", overview)
	b.WriteString("## Structure\n\n")

	for _, pkg := range model.Packages {
		name := pkg.Name
		if name == "" {
			name = "(default package)"
		}
		fmt.Fprintf(&b, "### %s\n\n", name)
		for _, t := range pkg.Types {
			fmt.Fprintf(&b, "- **%s** (%s)\n", t.Name, t.Kind)
		}
		b.WriteString("\n")
	}

	return b.String()
}
