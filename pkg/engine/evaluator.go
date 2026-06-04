package engine

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/media-luna/courtyard/pkg/git"
)

const templateEvalTimeout = 5 * time.Second

// Evaluate renders all template files using the provided values and returns
// the output files with resolved paths. All errors are collected before returning.
func Evaluate(ctx context.Context, spec *FormSpec, templateFiles map[string][]byte, values map[string]interface{}) ([]git.OutputFile, error) {
	ctx, cancel := context.WithTimeout(ctx, templateEvalTimeout)
	defer cancel()

	var files []git.OutputFile
	var errs []string

	for relPath, content := range templateFiles {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Evaluate: %w", ctx.Err())
		default:
		}

		outPath, err := resolveOutputPath(spec, relPath, values)
		if err != nil {
			errs = append(errs, fmt.Sprintf("resolve path %q: %v", relPath, err))
			continue
		}

		rendered, err := evalTemplate(relPath, content, values)
		if err != nil {
			errs = append(errs, fmt.Sprintf("render %q: %v", relPath, err))
			continue
		}

		files = append(files, git.OutputFile{Path: outPath, Content: rendered})
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("Evaluate: %s", strings.Join(errs, "; "))
	}
	return files, nil
}

// EvaluateString renders a single Go template string (e.g. BranchName, CommitMessage) with values.
func EvaluateString(tmplStr string, values map[string]interface{}) (string, error) {
	t, err := template.New("").Funcs(sprig.TxtFuncMap()).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("EvaluateString: parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("EvaluateString: execute: %w", err)
	}
	return buf.String(), nil
}

// resolveOutputPath determines the output path for a template file.
// It checks for a per-file TemplateOverride first, then falls back to
// combining spec.OutputPath with the file's relative path (sans .tmpl extension).
func resolveOutputPath(spec *FormSpec, relPath string, values map[string]interface{}) (string, error) {
	// Strip .tmpl extension for default path calculation.
	outName := relPath
	if strings.HasSuffix(outName, ".tmpl") {
		outName = strings.TrimSuffix(outName, ".tmpl")
	}

	// Check per-file override.
	if override, ok := spec.Templates[relPath]; ok && override.OutputPath != "" {
		return EvaluateString(override.OutputPath, values)
	}

	// Default: combine spec.OutputPath (evaluated as template) + outName.
	if spec.OutputPath == "" {
		return outName, nil
	}
	base, err := EvaluateString(spec.OutputPath, values)
	if err != nil {
		return "", err
	}
	return path.Join(base, outName), nil
}

// evalTemplate parses and executes a single Go template with Sprig functions.
func evalTemplate(name string, content []byte, values map[string]interface{}) ([]byte, error) {
	t, err := template.New(name).Funcs(sprig.TxtFuncMap()).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, values); err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}
	return buf.Bytes(), nil
}
