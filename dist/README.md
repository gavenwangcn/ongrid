# dist/ — ongrid release pipeline

This directory owns the **release/packaging** pipeline for ongrid. One command
produces one installation artefact; Compose runtime images are published and
pulled separately from the project CNB registry.

## What `make package` produces

A single tarball:

```
dist/out/ongrid-v<VERSION>-linux-amd64.tar.xz
dist/out/ongrid-v<VERSION>-linux-amd64.tar.xz.sha256
dist/out/ongrid-v<VERSION>-linux-arm64.tar.xz
dist/out/ongrid-v<VERSION>-linux-arm64.tar.xz.sha256
```

Unpacked layout:

```
ongrid-v<VERSION>-linux-<arch>/
  VERSION
  README.md              (from deploy/install/README.md)
  install.sh             (from deploy/install/install.sh)
  uninstall.sh
  upgrade.sh
  docker-compose.yml     (prod compose, from deploy/install/)
  .env.example
  prometheus/
    prometheus.yml       (from deploy/prometheus/)
  bin/
    ongrid               (systemd mode)
    ongrid-frontier      (systemd mode)
  edge/
    ongrid-edge-linux-amd64
    ongrid-edge-linux-arm64
    ongrid-edge-darwin-amd64
    ongrid-edge-darwin-arm64
    install-edge.sh
    ongrid-edge.yaml.example
    ongrid-edge.service
```

Compose mode does not embed image tarballs. `install.sh` renders the production
Compose model, pulls every exact image from `docker.cnb.cool/ongridio/ongrid`,
then runs `docker compose up -d`. Systemd binaries and Edge agents remain in
the architecture-specific package.

## Release flow

1. Bump the version: edit `VERSION` at the repo root (e.g. `v0.1.1`), commit
   the change, then tag that commit with the same value:
   `git tag v0.1.1 && git push origin v0.1.1`.
2. The `Release` GitHub Actions workflow runs on `v*.*.*` tag pushes and
   publishes the multi-architecture manager, Web, and Kubernetes Edge images
   plus the matching Helm chart before building both server packages. The chart is published as an
   OCI artifact at `oci://helm.cnb.cool/ongridio/ongrid-edge`; it is not copied
   into the manager installation tarball. Use
   `make package TARGET_ARCH=arm64` locally only when you need a single ARM64
   package. The release build will:
   - `docker-push-release-images` — publish manager, Web, and Edge amd64/arm64 images to CNB
   - `verify-release-images` — verify both architectures exist on all three image manifests
   - `publish-k8s-chart` — package and publish the version-matched Helm chart
   - `build-edge-all`    — cross-compile ongrid-edge for 4 targets
   - `package`           — extract systemd binaries from the published CNB images
   - stage everything under `dist/stage/ongrid-<VERSION>-linux-<arch>/`
   - emit the amd64/arm64 tarballs + sha256 files under `dist/out/`
3. Ship the matching package, for example:
   `scp dist/out/ongrid-v<VERSION>-linux-<arch>.tar.xz user@host:~/`.
4. On the target: untar, `sudo ./install.sh`.

## Checksum

`dist/out/ongrid-v<VERSION>-linux-<arch>.tar.xz.sha256` sits next to the
tarball. The install script can verify integrity with `sha256sum -c` on
Linux or `shasum -a 256 -c` on macOS.

## Local dry-run

Test the tarball without shipping after the matching manager image tag exists
in CNB (or override `CLOUD_MANAGER_IMAGE_REF` when calling `make`):

```
make package
mkdir -p /tmp/ongrid-test && tar -xf dist/out/ongrid-v*.tar.xz -C /tmp/ongrid-test
cd /tmp/ongrid-test/ongrid-v*
ls -R
# Inside a disposable Ubuntu container with docker socket mounted:
#   docker run --rm -it -v $PWD:/pkg -v /var/run/docker.sock:/var/run/docker.sock \
#     ubuntu:22.04 bash -c 'cd /pkg && ./install.sh'
```

## Files in this directory

- `package.sh` — assembly script invoked by `make package`. Tolerates missing
  `deploy/install/*` files (warns, continues) so the pipeline is testable
  before the on-target scripts land.
- `README.md` — this file.

## What this directory does NOT own

- `deploy/install/**` — on-target install/uninstall/upgrade scripts and prod
  `docker-compose.yml`. Owned by the install-agent.
- `deploy/Dockerfile.*`, `deploy/docker-compose.yml` — build contexts and dev
  compose file.
- Static third-party runtime images are mirrored out of band; this release
  pipeline only pushes the project's manager, Web, and Edge images.
