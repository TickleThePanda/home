---
paths:
  - "apps/**"
  - ".github/workflows/build--*"
---

# Images built from this repo

- `apps/<app>/` is the source for an image this repo builds. Its
  `.github/workflows/build--<app>.yml` triggers on `apps/<app>/**`, builds
  with that directory as the Docker context, pushes
  `ticklethepanda/<app>:latest`, and restarts the Deployment.
- Builds cover `linux/amd64` and `linux/arm64`, and fail if the pushed
  manifest is missing either.
- The manifests live under `deploy/` like any other app. Nothing there
  references `apps/`.
