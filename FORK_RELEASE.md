# DrNetwork release model

This is an independent, full-source repository—not a GitHub fork. Backend and frontend live together on `main`. Official S-UI changes are imported by the validated synchronization workflow, while DrNetwork branding, multi-node management and live traffic remain product-owned customizations.

Tags build Linux and Windows archives plus the multi-architecture container image. The installer, management script and checksum verification all resolve releases from `Danialrostamani/drnetwork-panel`.
