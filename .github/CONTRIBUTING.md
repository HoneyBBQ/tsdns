# Contributing to tsdns

First off, thank you for considering contributing to `tsdns`! It's people like you that make the open-source community such a great place.

## How Can I Contribute?

### Reporting Bugs
- Use the **Bug Report** template when opening an issue.
- Describe the bug in detail and provide steps to reproduce it.

### Suggesting Enhancements
- Use the **Feature Request** template.
- Explain why this enhancement would be useful to most users.

### Pull Requests
1. Fork the repository and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. Ensure the test suite passes (`make test`).
4. Make sure your code lints (`make lint`).
5. Use descriptive commit messages.

## Development Setup

### Prerequisites
- Go 1.23 or later
- `make`
- `golangci-lint` (for linting)

### Quality Checks
Before committing your changes, please run:
```bash
make fmt   # Format code
make lint  # Run static analysis
make test  # Run unit tests
```

## Style Guide
- Follow the standard Go coding conventions.
- All exported functions and types should have comments (we will enforce this as the project matures).
- Keep functions focused and small.

## License
By contributing, you agree that your contributions will be licensed under the MIT License.
