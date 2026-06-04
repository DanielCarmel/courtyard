---
description: "Frontend patterns for Courtyard UI. Use when working on HTML, CSS, or JavaScript in the ui/ directory. Covers Alpine.js usage, API integration, form rendering, and the no-build-step constraint."
applyTo: "ui/**"
---
# Frontend Guidelines

## Hard Constraints

- **No build step.** No npm, no webpack, no bundler, no transpiler.
- **No framework dependencies** beyond Alpine.js (loaded from CDN or vendored).
- **No TypeScript.** Plain ES2020+ JavaScript only.
- **Files served from `embed.FS`** — must work as static files from the Go binary.

## Alpine.js Usage

- Use `x-data` for component state on container elements
- Use `x-model` for two-way form binding
- Use `x-show` / `x-if` for conditional rendering
- Use `@change`, `@input`, `@click` for events
- Use `$watch` for debounced preview updates

## API Integration

- All API calls use `fetch()` with credentials: `fetch(url, { credentials: 'same-origin' })`
- Check response status: redirect to `/auth/github/login` on 401
- Parse JSON responses: `const data = await response.json()`
- Show error messages in the UI — never swallow errors silently
- Debounce preview calls: 500ms after last keystroke

## Form Rendering

The backend returns `FormSpec.Fields` as JSON. The frontend dynamically renders:
- `type: "string"` → `<input type="text">` with regex pattern if `validation` is set
- `type: "number"` → `<input type="number">`
- `type: "boolean"` → `<input type="checkbox">`
- `type: "enum"` → `<select>` with `<option>` for each item in `options`
- Apply `required` attribute from field spec
- Apply `default` value as initial form state

## Layout

- Two-panel layout: form on left, preview on right
- Mobile: stack vertically (form on top, preview below)
- Use CSS Grid or Flexbox — no CSS frameworks
- Monospace font for preview panel (`<pre>` blocks)
- Show file paths as tabs or headers in the preview panel

## Preview Flow

1. User edits form → debounce 500ms
2. `POST /api/repos/{owner}/{repo}/forms/{form}/preview` with JSON body of field values
3. Response: array of `{path, content}` objects
4. Render each file in the preview panel with its output path as header

## Submit Flow

1. User clicks Submit
2. `POST /api/repos/{owner}/{repo}/forms/{form}/submit` with same JSON body
3. On success: response contains `{prURL}` — show clickable link
4. On error: show error message (validation errors, branch conflicts, etc.)
5. Disable submit button during request to prevent double-submission
