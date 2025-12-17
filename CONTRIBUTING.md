# Contributing to Supreme Local Dev

Thank you for considering contributing to Supreme Local Dev!

## Getting Started

1.  **Fork the repository** on GitHub.
2.  **Clone your fork** locally:
    ```bash
    git clone https://github.com/YOUR_USERNAME/supreme-local-dev.git
    cd supreme-local-dev
    ```
3.  **Install Go dependencies**:
    ```bash
    go mod download
    ```

## Development Workflow

1.  Create a new branch for your feature or fix:
    ```bash
    git checkout -b feature/amazing-feature
    ```
2.  Make your changes.
3.  Test your changes by building and running the CLI:
    ```bash
    go build -o sld cmd/sld/main.go
    ./sld status
    ```
4.  Commit your changes following conventional commit messages if possible.

## Pull Requests

1.  Push your branch to GitHub.
2.  Open a Pull Request against the `main` branch.
3.  Describe your changes and why they are needed.

## Code Style

- Follow standard Go formatting (`go fmt`).
- Ensure code is readable and commented where necessary.
