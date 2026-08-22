# Requirements

- [Go](https://go.dev/dl/) (see the required version in the [go.mod](../../go.mod) file).
- [Node.js + npm](https://nodejs.org/en/download/) (check the required version in the [frontend/package.json](/frontend/package.json) file).
- [Wails](https://wails.io/docs/gettingstarted/installation).
  - We intentionally use one minor version behind latest for stability. It's recommended to install the matching version to avoid compatibility issues:
  ```bash
  # run from project root
  go install "github.com/wailsapp/wails/v2/cmd/wails@$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v2)"
  ```

  - After installation, run `wails doctor` to ensure you have the required dependencies.
- [Task](https://taskfile.dev/installation/) (optional, but recommended for quick access to common commands).
- [golangci-lint](https://golangci-lint.run/welcome/install/).
