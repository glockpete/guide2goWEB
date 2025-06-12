# Contributing to guide2goWEB

Thank you for your interest in contributing!

## How to Contribute

- Fork the repository and create your branch from `master`.
- Ensure your code follows Go best practices and is well-documented.
- Write unit tests for new features and bug fixes.
- Run `go test ./...` and ensure all tests pass before submitting a PR.
- Use structured logging and dependency injection patterns as in the codebase.
- Open a pull request with a clear description of your changes.

## Code Style

- Use `gofmt` and `goimports` to format your code.
- Use dependency injection and interfaces for testability.
- Prefer explicit error handling and context propagation.

## Testing

- All new code should include unit tests.
- Use mocks or interfaces for external dependencies.

## Pull Requests

- Keep PRs focused and small if possible.
- Reference related issues in your PR description.
- Be responsive to code review feedback.

## API Conventions

- Document all new API endpoints in the README under the API Endpoints section.
- Use RESTful conventions for endpoint naming and HTTP methods.
- Provide example requests and responses for new endpoints.
- Update the architecture diagram if you add major new components or flows.

Thank you for helping make guide2goWEB better! 