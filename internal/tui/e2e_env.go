package tui

import "os"

// NOTE: These env knobs are intended for deterministic E2E coverage only.
// They are production-safe because they are strictly opt-in.

func e2eStubHelm() bool {
	return os.Getenv("HELMDEX_E2E_STUB_HELM") == "1"
}

func e2eStubArtifactHub() bool {
	return os.Getenv("HELMDEX_E2E_STUB_ARTIFACTHUB") == "1"
}

func e2eNoEditor() bool {
	return os.Getenv("HELMDEX_E2E_NO_EDITOR") == "1"
}
