package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	semver "github.com/Masterminds/semver/v3"
)

// Universe is the set of charts this fake registry serves. Every repository URL
// resolves to the same universe: helmdex derives repo names by hashing the URL,
// so the URL carries no meaning for a stand-in registry.
type Universe struct {
	Charts []Chart `json:"charts"`
}

// Chart is one chart and the versions it publishes.
type Chart struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	AppVersion  string   `json:"appVersion"`
	Versions    []string `json:"versions"`

	// Optional per-chart artifact overrides. When empty, deterministic content
	// is generated from the chart name and version.
	Values string `json:"values,omitempty"`
	Readme string `json:"readme,omitempty"`
	Schema string `json:"schema,omitempty"`
}

// defaultUniverse mirrors fixtures/remote-source/catalog.yaml (postgresql
// 15.5.0, nginx 15.0.0) and publishes higher versions on top so upgrade flows
// have somewhere to go.
//
// postgresql-ha exists so that a search for "postgresql" returns a substring
// match too — helmdex filters those out by exact name, and that filter needs a
// case to filter.
func defaultUniverse() Universe {
	return Universe{Charts: []Chart{
		{
			Name:        "postgresql",
			Description: "Fake PostgreSQL chart",
			AppVersion:  "16.1.0",
			Versions:    []string{"15.4.0", "15.5.0", "15.6.0", "16.0.0", "16.1.0-rc.1"},
		},
		{
			Name:        "postgresql-ha",
			Description: "Fake PostgreSQL HA chart",
			AppVersion:  "16.1.0",
			Versions:    []string{"12.0.0", "12.1.0"},
		},
		{
			Name:        "nginx",
			Description: "Fake NGINX chart",
			AppVersion:  "1.25.3",
			Versions:    []string{"15.0.0", "15.1.0", "15.2.0"},
		},
		{
			// Matches the OCI dependency pinned by
			// fixtures/agnostic-gitops/apps/demo-preprod/Chart.yaml, whose
			// version is a CD-pipeline pseudo-version.
			Name:        "demo",
			Description: "Fake application chart",
			AppVersion:  "1.0.0",
			Versions:    []string{"0.0.0-sha.d68be124", "0.1.0", "0.2.0"},
		},
	}}
}

// loadUniverse returns the default universe, or the one described by the JSON
// file at HELMDEX_FAKE_HELM_SPEC when that variable is set.
func loadUniverse() (Universe, error) {
	specPath := strings.TrimSpace(os.Getenv(envSpec))
	if specPath == "" {
		return defaultUniverse(), nil
	}
	b, err := os.ReadFile(specPath)
	if err != nil {
		return Universe{}, fmt.Errorf("read %s=%s: %w", envSpec, specPath, err)
	}
	var u Universe
	if err := json.Unmarshal(b, &u); err != nil {
		return Universe{}, fmt.Errorf("parse %s=%s: %w", envSpec, specPath, err)
	}
	if len(u.Charts) == 0 {
		return Universe{}, fmt.Errorf("%s=%s declares no charts", envSpec, specPath)
	}
	return u, nil
}

func (u Universe) chart(name string) (Chart, bool) {
	for _, c := range u.Charts {
		if c.Name == name {
			return c, true
		}
	}
	return Chart{}, false
}

// sortedVersions returns the chart's versions newest-first, optionally
// including pre-releases.
func (c Chart) sortedVersions(devel bool) []string {
	out := make([]string, 0, len(c.Versions))
	for _, v := range c.Versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			// A non-semver version is a spec authoring mistake, not something to
			// paper over: surface it rather than silently dropping the version.
			fatalf("chart %s declares non-semver version %q: %v", c.Name, v, err)
		}
		if !devel && sv.Prerelease() != "" {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := semver.NewVersion(out[i])
		b, _ := semver.NewVersion(out[j])
		return a.GreaterThan(b)
	})
	return out
}

// resolveVersion picks the highest version satisfying constraint. An empty
// constraint means "highest stable release".
func (c Chart) resolveVersion(constraint string) (string, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		vs := c.sortedVersions(false)
		if len(vs) == 0 {
			return "", fmt.Errorf("chart %q has no stable version", c.Name)
		}
		return vs[0], nil
	}
	// An exact version wins outright, pre-release or not.
	for _, v := range c.Versions {
		if v == constraint {
			return v, nil
		}
	}
	cs, err := semver.NewConstraint(constraint)
	if err != nil {
		return "", fmt.Errorf("chart %q: bad version constraint %q: %w", c.Name, constraint, err)
	}
	for _, v := range c.sortedVersions(false) {
		sv, _ := semver.NewVersion(v)
		if cs.Check(sv) {
			return v, nil
		}
	}
	return "", fmt.Errorf("chart %q has no version matching %q", c.Name, constraint)
}

func (c Chart) valuesFor(version string) string {
	if c.Values != "" {
		return c.Values
	}
	return fmt.Sprintf(`# fake-helm default values for %s
replicaCount: 1
image:
  repository: fake/%s
  tag: "%s"
service:
  type: ClusterIP
  port: 8080
resources: {}
`, c.Name, c.Name, version)
}

func (c Chart) readmeFor(version string) string {
	if c.Readme != "" {
		return c.Readme
	}
	return fmt.Sprintf(`# %s

Fake chart served by fakehelm, version %s.

## Parameters

| Name | Description | Value |
| ---- | ----------- | ----- |
| `+"`replicaCount`"+` | Number of replicas | `+"`1`"+` |
`, c.Name, version)
}

func (c Chart) schemaFor(version string) string {
	if c.Schema != "" {
		return c.Schema
	}
	return fmt.Sprintf(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "%s %s",
  "type": "object",
  "properties": {
    "replicaCount": { "type": "integer", "default": 1, "description": "Number of replicas" },
    "image": {
      "type": "object",
      "properties": {
        "repository": { "type": "string" },
        "tag": { "type": "string" }
      }
    },
    "service": {
      "type": "object",
      "properties": {
        "type": { "type": "string", "enum": ["ClusterIP", "NodePort", "LoadBalancer"] },
        "port": { "type": "integer", "default": 8080 }
      }
    }
  }
}
`, c.Name, version)
}

func (c Chart) chartYAMLFor(version string) string {
	return fmt.Sprintf(`apiVersion: v2
name: %s
description: %s
type: application
version: %s
appVersion: "%s"
`, c.Name, c.Description, version, c.AppVersion)
}
