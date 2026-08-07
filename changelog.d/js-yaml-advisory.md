### Security
- **Bumped js-yaml to 4.3.1** (GHSA-5p4m-2wfm-xmqj) — resolves a high-severity quadratic-CPU advisory in `!!omap` resolution. Build tooling only: js-yaml is a transitive devDependency of ESLint and never reaches the browser bundle, so no deployed instance was exposed.
