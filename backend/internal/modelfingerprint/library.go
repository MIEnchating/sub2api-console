// Package modelfingerprint plans and evaluates comparisons against pinned public
// reference data. It never sends requests, reads credentials or certifies identity.
package modelfingerprint

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed data/library.json
var libraryJSON []byte

const Version = "public-fingerprint-2026-10-16-v1"

type Source struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
	License  string `json:"license"`
}
type domain struct {
	Tolerance float64    `json:"tolerance"`
	Mode      string     `json:"tolerance_mode"`
	Range     [2]float64 `json:"value_range"`
	Template  string     `json:"cloze_template"`
}
type Question struct {
	Name   string  `json:"name"`
	Domain string  `json:"domain"`
	Value  float64 `json:"value"`
}
type knowledge struct {
	Provider       string     `json:"provider"`
	Date           string     `json:"date"`
	System         string     `json:"system"`
	SelfTotal      int        `json:"self_total"`
	SelfMismatches int        `json:"self_mismatches"`
	Questions      []Question `json:"questions"`
}
type tokenizer struct {
	Provider string         `json:"provider"`
	Date     string         `json:"date"`
	Version  string         `json:"version"`
	ProbeSet string         `json:"probe_set"`
	Values   map[string]int `json:"values"`
}
type distributionCell struct {
	ID      string             `json:"id"`
	Prompt  string             `json:"prompt"`
	Valid   int                `json:"valid"`
	Dist    map[string]float64 `json:"dist"`
	Maximum int                `json:"maximum"`
}
type distribution struct {
	Date  string             `json:"date"`
	Cells []distributionCell `json:"cells"`
}
type profile struct {
	Model        string        `json:"model"`
	Aliases      []string      `json:"aliases"`
	Sources      []Source      `json:"sources"`
	KBF          *knowledge    `json:"kbf"`
	Tokenizer    *tokenizer    `json:"tokenizer"`
	Distribution *distribution `json:"distribution"`
	Unavailable  string        `json:"unavailable"`
}
type library struct {
	Version      string            `json:"version"`
	Template     string            `json:"kbf_template"`
	Domains      map[string]domain `json:"domains"`
	TokenPrompts map[string]string `json:"token_prompts"`
	Profiles     []profile         `json:"profiles"`
}

var data = func() library {
	var result library
	if err := json.Unmarshal(libraryJSON, &result); err != nil {
		panic(err)
	}
	if result.Version != Version {
		panic("public fingerprint version mismatch")
	}
	return result
}()

func lookup(model string) *profile {
	// Only explicitly registered spellings; never strip dates, suffixes or arbitrary prefixes.
	for i := range data.Profiles {
		for _, alias := range data.Profiles[i].Aliases {
			if strings.TrimSpace(model) == alias {
				return &data.Profiles[i]
			}
		}
	}
	return nil
}

type CatalogEntry struct {
	Model       string   `json:"model"`
	Aliases     []string `json:"aliases"`
	Requests    int      `json:"requests"`
	Methods     []string `json:"methods"`
	Unavailable string   `json:"unavailable,omitempty"`
}

func Catalog() []CatalogEntry {
	result := make([]CatalogEntry, 0, len(data.Profiles))
	for _, p := range data.Profiles {
		methods := []string{}
		if p.KBF != nil {
			methods = append(methods, "knowledge")
		}
		if p.Tokenizer != nil {
			methods = append(methods, "tokenizer")
		}
		if p.Distribution != nil {
			methods = append(methods, "distribution")
		}
		result = append(result, CatalogEntry{p.Model, append([]string(nil), p.Aliases...), RequestCount(p.Model), methods, p.Unavailable})
	}
	return result
}
func Digest() string { sum := sha256.Sum256(libraryJSON); return hex.EncodeToString(sum[:]) }
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Counts are immutable after startup. Task progress must not rebuild hundreds
// of prompt strings for every active combination on every progress event.
var requestCounts = func() map[string]int {
	counts := map[string]int{}
	for _, p := range data.Profiles {
		n := len(Plan(p.Model))
		for _, alias := range p.Aliases {
			counts[alias] = n
		}
	}
	return counts
}()

func RequestCount(model string) int { return requestCounts[strings.TrimSpace(model)] }
