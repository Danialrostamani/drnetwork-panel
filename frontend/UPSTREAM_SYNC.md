# Upstream synchronization

`multinode` is the maintained product branch. `.github/workflows/upstream-sync.yml` checks the official `alireza0/s-ui-frontend` repository every day, merges new commits, validates lint/typecheck/tests/build, and pushes only a successful update. A merge conflict opens a GitHub issue and leaves the branch unchanged.
