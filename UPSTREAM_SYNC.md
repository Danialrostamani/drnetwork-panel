# Official source synchronization

DrNetwork is a standalone repository: the complete backend and frontend source trees are committed here and there are no submodules or runtime dependencies on a fork.

`.github/workflows/upstream-sync.yml` runs daily and can also be started manually. It:

1. merges new commits from `alireza0/s-ui`;
2. restores the vendored `frontend/` directory in place of the official backend's gitlink;
3. applies the delta from `alireza0/s-ui-frontend` to the vendored frontend;
4. normalizes Go imports to the DrNetwork module path;
5. runs backend and frontend validation; and
6. pushes only a fully validated update.

The exact imported official frontend commit is recorded in `.upstream/frontend.commit`. A real customization conflict stops the workflow without changing `main`, so it can be reviewed safely.
