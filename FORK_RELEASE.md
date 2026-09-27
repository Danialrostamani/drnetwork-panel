# Multi-node release channel

The maintained product branch is `multinode`. Official S-UI changes are merged into it by the daily upstream-sync workflow, while the frontend remains pinned to `Danialrostamani/s-ui-frontend-multinode`.

## Install or update

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/s-ui-multinode/multinode/install.sh)
```

The installed `s-ui` command, version-specific installs, checksums, and future updates all resolve releases from `Danialrostamani/s-ui-multinode`; they never replace this build with the official upstream binary.

## Release

Push a version tag such as `v1.6.3-multinode.1`. The release workflow builds the pinned frontend and all supported binaries, publishes `SHA256SUMS`, and creates a stable GitHub release consumed by the installer.

The Docker workflow publishes the same tagged source to:

```text
ghcr.io/danialrostamani/s-ui-multinode:<tag>
```
