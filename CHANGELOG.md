# Changelog

## v20260608 (2026-06-08)

### Added
- agent working agreement (AGENTS.md) per house standard (8068e77)
- extended attributes in the WinFsp backend mapped to NTFS alternate data streams (xattr.<name>), full set/get/list/remove with XATTR_CREATE/XATTR_REPLACE semantics and stream enumeration via FindFirstStreamW + symbolic link support: Getattr now uses lstat semantics and reports S_IFLNK, Readlink/Symlink passthrough (EPERM without Developer Mode), hard links via os.Link on the source volume * errno mapping recognizes ERROR_PRIVILEGE_NOT_HELD as EPERM (e5570f7)
- Statfs in the WinFsp backend reporting the source volume's capacity via GetDiskFreeSpaceEx so Explorer shows real totals instead of zeros * golang.org/x/sys becomes a direct dependency, bumped to v0.45.0 (raises the Go directive to 1.25, still within the supported oldstable/stable window CI tracks) (7128d1e)
- publish the Docker image to GHCR: moving :nightly tag from nightly.yml, :latest plus dated tag from release.yml * README: working install instructions (go install, ghcr.io image), Windows/WinFsp usage section, honest platform support (4068105)
- Windows support: new pkg/winfs backend on WinFsp via cgofuse (pure Go, no cgo) reusing the same pattern/filter core as the FUSE backend with identical blacklist, read-only and hidden-children semantics * cmd/filterfs split into platform-neutral CLI plus per-platform mount implementations; FUSE backend and integration tests are now linux/darwin build-tagged + windows-amd64/arm64 release binaries in build-all + windows CI job: required build+vet+unit tests, advisory WinFsp mount smoke test (4db6eac)
- adopt the house-standard CI/CD/release cycle: manual dispatch release.yml cutting dated vyyyyMMdd releases, automatic nightly-yyyyMMdd prereleases on green CI with GFS pruning, shared _build.yml producing the multi-platform binaries and Docker image, changelog generated from prefixed commit subjects * ci.yml is now the reusable test-only workflow (name CI, workflow_call) - build/release jobs and the tag trigger moved into the standard cycle + CHANGELOG.md seeded for update-changelog.mjs * README: correct repo URLs (Hawkynt/FilterFilesystem instead of the fictional filterfs org), document the release/nightly scheme and the actual platform support (01b37fb)
- initial commit (716b765)

### Changed
- mirror current changelog/prune scripts from the shared template (notes-only mode, bot-commit filtering, newest-kept guard on top of the existing list-retry) * nightly: same-day re-runs replace release AND tag at the validated SHA * release: the vyyyyMMdd tag is placed ON the changelog commit so bookkeeping never pollutes the next notes (f0cc6d2)
- README: blockquote intro, mapped emoji headers, ship-row badges, funding support section separated from getting-help, house license wording # stale filterfs/filterfs upstream links replaced with this repo's own (9e8500e)
- module path now matches the repository (github.com/Hawkynt/FilterFilesystem) so 'go install' actually works * go directive 1.22 which also enables the copyloopvar linter (1825c6f)
- migrate lint config to the golangci-lint v2 schema (version 2, linters.settings, exclusions, formatters section) - drop linters removed upstream (deadcode, golint, interfacer, scopelint, structcheck, varcheck) and the rule-less depguard no-op * exportloopref/scopelint replaced by copyloopvar, gomnd renamed to mnd, gofmt/goimports moved to formatters (1559e50)
- adopt the existing Go test workflow under the standard ci.yml name and add the Node 24 actions env - remove the duplicate generic ci.yml (the repo's FUSE-aware Go CI is kept) * standardize the README badge block to the house style (5d6f934)

### Fixed
- Release badge dropped sort=semver so it shows the latest stable release (matching the link) instead of ranking nightly/legacy tags highest (d15efe1)
- nightly badge: include_prereleases must be a valueless shields.io flag — =true renders 'invalid query parameter' (81f8103)
- nightly prune aborted the whole job when a transient GitHub API 504 hit 'gh release list' - retry three times with a pause, then skip the prune gracefully since the next nightly catches up anyway (b59414f)
- windows CI build picked cgofuse's cgo path because the runner ships gcc and then failed on missing FUSE headers - force CGO_ENABLED=0 so the runtime WinFsp DLL path is used (384f6fb)
- Docker build failed because filterfs.example.yaml was copied from the builder stage but never generated there - run 'make example-config' during the build * track current golang:alpine builder image instead of the EOL 1.21 pin (dfad8f9)
- darwin cross-build broke because syscall.Stat_t.Mode is uint16 there - restore the explicit uint32 conversions with lint annotations for the linux-targeted analysis - windows target from build-all since go-fuse has no Windows port + linux-arm64 release binary (8978796)
- unprivileged mounts failed because allow_other was hardcoded while fusermount requires user_allow_other in /etc/fuse.conf for that * allow_other is now the opt-in --allow-other flag (default off) and documented in the README (f68ec1a)
- opening existing files with O_TRUNC failed with 'operation not supported' because the kernel's truncate arrives as SETATTR which was unimplemented + Setattr on FilterNode handling size, mode, ownership and timestamps with read-only enforcement (77ea5ba)
- file operations in the mount root hit go-fuse defaults (EROFS on create, unfiltered fallback listing, broken read-only enforcement) because the root inode type only implemented Getattr/Lookup - mount a regular FilterNode with empty relative path as root instead and demote FilterFS to a plain shared-state holder - duplicated root-only Getattr/Lookup implementations (8fa29fa)
- code did not compile: os.EOF does not exist (now io.EOF), unused 'newPath' in Rename, unused imports in the integration test # pattern matcher used filepath.Match whose wildcard semantics depend on the host OS separator - on Windows '*' crossed '/' boundaries making first-sublevel patterns match at any depth; switched to path.Match since all paths are slash-normalized * conform to the lint set: extract prepareMountPoint/mountOptions from runMount, context-aware exec for unmount helpers, appName/mountPointPerm constants, wrapped long FUSE signatures, dropped redundant uint32 conversions, gofmt/goimports formatting across all files * lint config: exclusion rules need two criteria at runtime - tie message excludes to staticcheck; skip goconst/gosec in test files (7c90f62)
- go.sum contained fabricated checksums (testify, zap) so every 'go mod download' aborted with a security error - regenerate it entirely from the module proxy (7a8befb)
- quote Go matrix versions so YAML no longer collapses 1.20 into Go 1.2.2 * track oldstable/stable Go instead of pinned EOL versions * bump actions to Node-24-native majors (checkout v6, setup-go v6, cache v5, upload-artifact v7, codecov v5, golangci-lint-action v8) * replace the archived create-release/upload-release-asset actions with gh release create # trigger the workflow on v* tags and let the build job run for tag pushes so the release job is reachable + explicit workflow permissions (contents read, write only for release) - drop the FORCE_JAVASCRIPT_ACTIONS_TO_NODE24 workaround now that all actions are Node-24-native (8efdca4)
- example-config heredoc body was parsed by Make itself, aborting every target with 'missing separator' at line 103 - emit the YAML from a define/export block via printf instead (fedf7e4)

All notable changes are recorded here. This file is maintained automatically by
`.github/workflows/scripts/update-changelog.mjs`, which bucketises commits by
their prefix (`+` added, `*` changed, `-` removed, `#` fixed).

## [Unreleased]

- Initial repository setup.
