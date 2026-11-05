package project

import (
	"encoding/xml"
	"os"
	"path/filepath"
)

// BuildSystem identifies which build tool a project uses, if any.
type BuildSystem string

const (
	BuildSystemNone   BuildSystem = "none"
	BuildSystemMaven  BuildSystem = "maven"
	BuildSystemGradle BuildSystem = "gradle"
)

// BuildInfo is the lightweight build-system metadata used to seed the
// project-level README (name, and which tool produced the project layout).
// Deliberately shallow for MVP: full dependency-tree parsing is out of
// scope, this only answers "what is this project called".
type BuildInfo struct {
	System BuildSystem
	Name   string // best-effort project name; falls back to the root directory's basename
}

type pomXML struct {
	ArtifactID string `xml:"artifactId"`
	Name       string `xml:"name"`
}

// DetectBuildSystem inspects root for a pom.xml or a Gradle build file and
// returns what it can determine. It never errors — a project with neither
// (or with a malformed build file) still gets a usable BuildInfo via the
// directory-name fallback.
func DetectBuildSystem(root string) BuildInfo {
	if info, ok := detectMaven(root); ok {
		return info
	}
	if info, ok := detectGradle(root); ok {
		return info
	}
	return BuildInfo{System: BuildSystemNone, Name: filepath.Base(root)}
}

func detectMaven(root string) (BuildInfo, bool) {
	path := filepath.Join(root, "pom.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		return BuildInfo{}, false
	}

	var pom pomXML
	name := filepath.Base(root)
	if xml.Unmarshal(data, &pom) == nil {
		if pom.Name != "" {
			name = pom.Name
		} else if pom.ArtifactID != "" {
			name = pom.ArtifactID
		}
	}

	return BuildInfo{System: BuildSystemMaven, Name: name}, true
}

func detectGradle(root string) (BuildInfo, bool) {
	for _, candidate := range []string{"build.gradle", "build.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
			// Gradle project names conventionally live in settings.gradle's
			// rootProject.name; parsing Groovy/Kotlin DSL is out of scope for
			// MVP, so this falls back to the directory name.
			return BuildInfo{System: BuildSystemGradle, Name: filepath.Base(root)}, true
		}
	}
	return BuildInfo{}, false
}
