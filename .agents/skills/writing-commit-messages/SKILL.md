---
name: writing-commit-messages
description: >-
  Writes Git commit messages for Zen. Use when asked to commit changes,
  write or draft a commit message, or similar.
---

# Writing Commit Messages

## Format

```
<prefix>: <summary>

<long form description>

<issue reference>
```

## Rules

### Subject line

- **Prefix**: the repo path of the area changed, determined from the file paths in the diff:
  - Go package path relative to the repo root, e.g. `internal/sysproxy`, `internal/networkrules`. Nest deeper when the change is confined to a subpackage, e.g. `internal/asset/scriptlet`.
  - `frontend` for UI changes, without deeper nesting.
  - The directory for other areas, e.g. `docs`, `scripts`, `.github/workflows`.
  - The filename for changes to a single top-level file, e.g. `go.mod`, `wails.json`.
  - For a change spanning two areas, list both, comma-separated, e.g. `internal/proxy, internal/sysproxy`.
  - `all` for cross-cutting changes spanning three or more areas.
- **Summary**: completes the sentence "this change modifies Zen to \_\_\_". Imperative mood, lowercase start, no trailing period. Keep the whole subject line under ~72 characters.
- Two closely related changes can be joined with `,`, e.g. `all: update copyright year to 2026, update copyright holders`.

### Long form description

- Full sentences with normal capitalisation and punctuation, unlike the subject line.
- One to three short paragraphs of plain prose, no Markdown, no bullet points. Do not hard-wrap lines.
- Focus on the why; do not restate the diff.
- Suggested format (adapt to the context): describe what changed, what the previous behaviour was, and why the new behaviour is correct.
- Omit the body entirely for trivial or mechanical changes.

### Issue reference

- If the change closes an issue, put `Close #NNN` on the last line, separated from the body by a blank line.
- If it relates to an issue without closing it, use `Update #NNN` instead.
- If there is no related issue, omit this section entirely.

## Workflow

1. Run `git status` and `git diff` (plus `git diff --staged`) to see what is changing since the last commit.
2. Determine the prefix from the changed file paths.
3. Identify related issues from the diff context.
4. Draft the message following the format above.
5. For a draft-only request, stop after Step 4.
6. For an explicit commit request, stage only the files belonging to the change and commit.
7. Do not push, leave that to the user.
