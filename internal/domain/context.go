package domain

import (
	"regexp"
	"strings"
)

// ContextSignals is the Context Analyzer output: a plain, immutable struct with
// the contextual dimensions of a request that downstream layers (Complexity /
// Policy) use to size the routing decision. It holds only derived counts, sizes,
// and a detected language label — never prompts, bodies, or credentials.
type ContextSignals struct {
	// FileCount is how many distinct files are referenced in the context.
	FileCount int
	// Language is the dominant programming language detected, or "" when unknown.
	Language string
	// ContextSize is the total character length of the analyzed context text.
	ContextSize int
	// ToolCount is how many distinct tools/functions are referenced.
	ToolCount int
	// HistoryTurns is the number of conversation messages in the context.
	HistoryTurns int
	// DiffSize is the character length of unified-diff content detected.
	DiffSize int
}

// ContextInput carries the raw material the Context Analyzer inspects. It mirrors
// what the router already has (extracted prompt text, request body, message
// count, and referenced tool/file names) without importing host or ABI types, so
// the domain layer stays pure and testable.
type ContextInput struct {
	// Prompt is the extracted user text (e.g. from ExtractUserPrompt).
	Prompt string
	// Body is the raw request text used for diff and language heuristics.
	Body string
	// HistoryTurns is the number of messages in the conversation, when known.
	HistoryTurns int
	// Files lists file paths/names referenced by the request, when known.
	Files []string
	// Tools lists tool/function names referenced by the request, when known.
	Tools []string
}

var (
	// diffLineRe matches unified-diff added/removed content lines (not the +++/---
	// file headers), used to size the diff portion of a context.
	diffLineRe = regexp.MustCompile(`(?m)^[+-][^+-].*$`)
	// filePathRe extracts file-like tokens (path/name.ext) to count referenced
	// files when an explicit Files list is not provided.
	filePathRe = regexp.MustCompile(`[\w./-]+\.[A-Za-z0-9]{1,8}`)
)

// languageByExtension maps common source file extensions to a language label.
var languageByExtension = map[string]string{
	"go":    "go",
	"py":    "python",
	"js":    "javascript",
	"jsx":   "javascript",
	"ts":    "typescript",
	"tsx":   "typescript",
	"java":  "java",
	"rb":    "ruby",
	"rs":    "rust",
	"c":     "c",
	"h":     "c",
	"cpp":   "cpp",
	"cc":    "cpp",
	"hpp":   "cpp",
	"cs":    "csharp",
	"php":   "php",
	"swift": "swift",
	"kt":    "kotlin",
	"scala": "scala",
	"sh":    "shell",
	"sql":   "sql",
	"yaml":  "yaml",
	"yml":   "yaml",
	"json":  "json",
	"html":  "html",
	"css":   "css",
	"md":    "markdown",
}

// AnalyzeContext inspects a request context and returns its ContextSignals. It is
// pure (no I/O, no host/ABI dependencies) and safe to call on any request.
func AnalyzeContext(in ContextInput) ContextSignals {
	text := in.Body
	if strings.TrimSpace(text) == "" {
		text = in.Prompt
	}

	files := dedupeLower(in.Files)
	if len(files) == 0 {
		files = extractFileTokens(text)
	}

	return ContextSignals{
		FileCount:    len(files),
		Language:     detectLanguage(files, text),
		ContextSize:  len(text),
		ToolCount:    len(dedupeLower(in.Tools)),
		HistoryTurns: in.HistoryTurns,
		DiffSize:     diffSize(text),
	}
}

// detectLanguage picks the dominant language from referenced file extensions,
// falling back to a lightweight keyword scan of the text. Returns "" when unknown.
func detectLanguage(files []string, text string) string {
	counts := make(map[string]int)
	for _, file := range files {
		if lang := languageForFile(file); lang != "" {
			counts[lang]++
		}
	}
	if lang := topLanguage(counts); lang != "" {
		return lang
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "package main") || strings.Contains(lower, "func "):
		return "go"
	case strings.Contains(lower, "def ") || strings.Contains(lower, "import numpy"):
		return "python"
	case strings.Contains(lower, "function ") || strings.Contains(lower, "const ") && strings.Contains(lower, "=>"):
		return "javascript"
	default:
		return ""
	}
}

// languageForFile returns the language label for a file's extension, or "".
func languageForFile(file string) string {
	idx := strings.LastIndex(file, ".")
	if idx < 0 || idx == len(file)-1 {
		return ""
	}
	ext := strings.ToLower(file[idx+1:])
	return languageByExtension[ext]
}

// topLanguage returns the most frequent language, breaking ties alphabetically
// for determinism. Returns "" for an empty map.
func topLanguage(counts map[string]int) string {
	best := ""
	bestCount := 0
	for lang, count := range counts {
		if count > bestCount || (count == bestCount && (best == "" || lang < best)) {
			best = lang
			bestCount = count
		}
	}
	return best
}

// extractFileTokens finds distinct file-like tokens in text when no explicit file
// list is provided.
func extractFileTokens(text string) []string {
	matches := filePathRe.FindAllString(text, -1)
	return dedupeLower(matches)
}

// diffSize returns the total length of unified-diff content lines in text.
func diffSize(text string) int {
	if !strings.Contains(text, "@@") && !strings.Contains(text, "diff --git") &&
		!strings.Contains(text, "\n+") && !strings.Contains(text, "\n-") {
		return 0
	}
	total := 0
	for _, line := range diffLineRe.FindAllString(text, -1) {
		total += len(line)
	}
	return total
}

// dedupeLower lowercases, trims, drops empties, and removes duplicates preserving
// first-seen order.
func dedupeLower(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
