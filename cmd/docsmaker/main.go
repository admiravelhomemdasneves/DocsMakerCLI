// Command docsmaker is the entrypoint for DocsMakerCli.
package main

import (
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cli"

	// Blank-imported so its init() registers the Java LanguageAnalyzer.
	// This is the only place in the codebase that needs to know Java exists —
	// internal/app and internal/cli talk to langmodel.LanguageAnalyzer only.
	_ "github.com/admiravelhomemdasneves/DocsMakerCLI/internal/lang/java"
)

func main() {
	cli.Execute()
}
