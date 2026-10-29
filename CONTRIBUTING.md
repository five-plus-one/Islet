# Contributing to Islet

Report reproducible bugs and discuss features through GitHub Issues. For security issues, use the private reporting process in SECURITY.md.

Use Go 1.26.5+ and Node.js 22+. Install frontend dependencies with npm ci in frontend/admin. Run go test ./... in backend, then npm run type-check and npm run build in frontend/admin. Docker builds use backend/Dockerfile with the repository root as context.

Keep business behavior in Islet and reusable UI in the shared component library. Explain the user-facing problem and test results in each pull request. Never include credentials, personal image data or private deployment configuration.

Contributions to Islet are submitted under GPL-3.0-only. Third-party components retain their own licenses.
