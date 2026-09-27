# Upstream synchronization

`multinode` is the maintained product branch. `.github/workflows/upstream-sync.yml` checks `alireza0/s-ui` daily, merges new commits, updates the frontend submodule from `Danialrostamani/s-ui-frontend-multinode`, runs the Go validation suite, and pushes only when validation succeeds. Conflicts open a GitHub issue and leave the branch unchanged.
