# Project structure

This document describes the file structure of the project.

- `.github`: GitHub Actions workflows and issue templates.
- `assets`: Assets used in README.md and other documents (not in source files).
- `build`: Build configuration and artifacts. Read more in [build/README.md](../../build/README.md).
- `docs`: Project documentation.
- `frontend`: Frontend JS/TS code. Read more in [frontend/README.md](../../frontend/README.md).
- `internal`: Backend Go packages.
- `scriptlets`: JS/TS functions injected into webpages for advanced content blocking.
- `scripts`: Node.js scripts for manifest file uploads. May be used for other purposes in the future.
- `tasks`: Platform-specific build-related [Taskfiles](https://taskfile.dev).
- `main.go`: The main entry point of the application.
- `golangci.yml`: Configuration file for [golangci-lint](https://golangci-lint.run).
- `Taskfile.yml`: Main [Taskfile](https://taskfile.dev) for common development tasks.
- `wails.json`: [Wails](https://wails.io) configuration file.
