# Release process

How MYTHHELM versions, tags and ships releases. The scheme below is implemented by the release pipeline described in the Pipeline section at the end of this document.

## Scheme

Releases follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html), ratified in [ADR 0009](decisions/0009-version-numbering-scheme.md).

Example version strings: `v0.1.0` for an S1-style preview release, `v1.2.3` for a later stable release. The shapes are illustrative of the scheme; the exact tag format is owned by MH-19, whose tag format and mechanics carry this scheme.

Scope is release versions only; dev strings such as unstamped-build markers are out of scope and continue untouched. MH-7 implements the tag-triggered GoReleaser pipeline under this scheme. MH-7's spec extends this document with the pipeline sections; nothing here assumes pipeline details MH-7 has not designed.

## Tag discipline

Tag format is `v<semver>` carrying the scheme above, with dotted pre-release markers (MH-19 discipline, [ADR 0010](decisions/0010-release-tag-discipline.md)). Valid examples are `v0.1.0` and `v1.2.3-rc.1`. Invalid examples are `0.1.0` (missing `v`), `v1.2` (incomplete SemVer) and `v1.2.3-rc1` (undotted marker, invalid per project convention, though bare SemVer would allow it; the convention is stricter than the specification and never mistaken for it).

The operator-publishes mechanism applies: only the operator creates release tags, and only the operator publishes. The maintainer holding release authority publishes the release, which creates the tag, never by hand and never via `git tag` by hand. Required permission is release write (release-write: the Contents: write access that covers release publication — GitHub exposes no separate release permission); tag creation rides the release write. Publishing runs `gh release edit vX.Y.Z --draft=false --target <merge>`, the finalize-spec-tag flow, where publishing the draft creates the tag at the merge.

Observed settings are read-only and unchanged by this task. Command run on 2026-10-06 (read-only, settings unchanged):

```text
gh api repos/turbokast/mythhelm/rulesets
[{"id":24186707,"name":"Protect main","target":"branch","source_type":"Repository","source":"turbokast/mythhelm","enforcement":"active","node_id":"RRS_lACqUmVwb3NpdG9yec5TLnMAzgFxD1M","_links":{"self":{"href":"https://api.github.com/repos/turbokast/mythhelm/rulesets/24186707"},"html":{"href":"https://github.com/turbokast/mythhelm/rules/24186707"}},"created_at":"2026-09-29T15:20:24.522+01:00","updated_at":"2026-09-29T19:53:47.868+01:00"}]
```

The listed `Protect main` ruleset is the observed baseline. Any ruleset or tag-protection change stays an operator action.

Trigger contract for MH-7 to implement: Pushing a release tag starts the release pipeline, with publication creating `vX.Y.Z` at the target merge and the tag push matching the `v*` filter triggering the pipeline. No release artifact ships without its tag, since the pipeline requires tag context. Failure rule: a pipeline run from a malformed or unauthorized tag fails closed with no partial release, where non-matching tags never match the trigger filter and an aborted run publishes nothing.

Release scope for tagging: every published release including pre-releases gets a tag, with the dotted marker rule above; drafts are never tagged, since a draft is not a release. A pre-release draft is marked as a prerelease before publishing, so the published RC carries GitHub's prerelease flag and never appears as stable.

## Pipeline

Pushing a tag that matches `v*` starts `.github/workflows/release.yml`. The operator creates release tags only by publishing the draft release (see Tag discipline). A hand-pushed tag matching `v*` still triggers the workflow and queues for `release` environment approval; once approved, the job's first step rejects any name that does not match `v<major>.<minor>.<patch>` (no leading zeroes) optionally followed by `-` and two or more dot-separated alphanumeric pre-release identifiers, before checkout or any build. The check does not apply SemVer's leading-zero rule to numeric pre-release identifiers, so `v1.2.3-rc.01` passes it.

### What a release builds

GoReleaser builds a five-pair matrix: linux/amd64, linux/arm64, darwin/arm64, windows/amd64 and windows/arm64. darwin/amd64 is excluded. Each pair yields one archive (`.tar.gz` for linux and darwin, `.zip` for windows) holding the binary, `LICENSE`, `NOTICE` and `third_party_licenses/`, plus a per-archive SPDX SBOM (`<archive>.sbom.json`). The release also carries `checksums.txt` and its cosign bundle, `checksums.txt.sigstore.json`.

The job runs in the `release` environment. Observed state (Q2, settings unchanged by this document): the environment has a required reviewer and a `v*` tag deployment policy, so the job waits for approval and no other ref can start it.

### Operator publish flow

1. Check that the release-drafter draft names the intended version and notes.
2. Publish the draft, which creates the tag at the merge:

   ```bash
   gh release edit vX.Y.Z --draft=false --target <merge>
   ```

   Never create the tag with `git tag`.
3. Approve the pending `release` environment deployment on the `Release` workflow run.
4. When the run is green, the release has the five archives, five SBOMs, `checksums.txt` and `checksums.txt.sigstore.json`, and build-provenance attestations over `checksums.txt`, `dist/*.tar.gz` and `dist/*.zip`.

### Verify a release

Download the assets, then run the three checks from the download directory. Set `TAG` to the release tag (for example `v1.2.3`) and `ARCHIVE` to the archive you downloaded.

```bash
TAG=vX.Y.Z
ARCHIVE=mythhelm_X.Y.Z_linux_amd64.tar.gz
gh release download "$TAG" --repo turbokast/mythhelm --pattern "$ARCHIVE" --pattern checksums.txt --pattern checksums.txt.sigstore.json

# 1. Checksum
sha256sum -c --ignore-missing checksums.txt

# 2. Build-provenance attestation
gh attestation verify "$ARCHIVE" --repo turbokast/mythhelm

# 3. Cosign signature over checksums.txt
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/turbokast/mythhelm/.github/workflows/release.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

Check 1 and check 3 together prove the archive matches a checksum list that the release workflow signed.

Tamper leg: append a byte to the archive and run check 1 again; `sha256sum -c` reports `FAILED` and exits non-zero. `gh attestation verify` on the modified archive also fails.

```bash
printf x >> "$ARCHIVE"
sha256sum -c --ignore-missing checksums.txt   # expected: FAILED, exit 1
```

### Dry run on a test tag

`.github/workflows/release-dry-run.yml` runs on tags matching `test/*` and publishes nothing: it has no environment, no secret and read-only repository access. The operator pushes the tag at the pull request head or at `main`, for example `test/2026-10-08-mh7`. The run builds a snapshot of the same matrix, checks the archives, SBOMs and notices payload, signs `checksums.txt` with cosign and verifies the bundle, then uploads `dist/` as the artifact `dist` (7-day retention). Attestation does not run in the dry run, so check 2 does not apply to it.

To verify a dry-run artifact, download it with `gh run download <run-id> --name dist --dir dist`, run from an empty working directory. The bundle's signing identity is the dry-run workflow, so check 3 uses the dry-run identity (set `TAG` to the full test tag, including `test/`). Delete the test tag afterwards.

```bash
TAG=test/YYYY-MM-DD-mh7
(cd dist && sha256sum -c checksums.txt)
cosign verify-blob \
  --bundle dist/checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/turbokast/mythhelm/.github/workflows/release-dry-run.yml@refs/tags/$TAG" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  dist/checksums.txt
```

### Re-run after a late-step failure

GoReleaser publishes the release before the cosign and attestation steps run, so a failure in a later step leaves a published release without its bundle or attestation. Check what is missing with `gh release view "$TAG" --repo turbokast/mythhelm` (is `checksums.txt.sigstore.json` listed?) and `gh attestation verify "$ARCHIVE" --repo turbokast/mythhelm`. Then recover from the failed run:

1. Remove the GoReleaser assets from the release first: the five archives, their `.sbom.json` files and `checksums.txt`. `.goreleaser.yml` sets `release.mode: keep-existing` without replacing existing artifacts, so the re-run's GoReleaser step does not replace the published assets; it collides with them and fails. The bundle is handled in step 2.

   ```bash
   for a in $(gh release view "$TAG" --repo turbokast/mythhelm --json assets --jq '.assets[].name' | grep -E '^(mythhelm_.*\.(tar\.gz|zip|sbom\.json)|checksums\.txt)$'); do
     gh release delete-asset "$TAG" "$a" --yes --repo turbokast/mythhelm
   done
   ```

2. The workflow's upload step does not overwrite either. If the bundle is already on the release, that step stops the re-run before the attestation; remove the stale bundle too:

   ```bash
   gh release delete-asset "$TAG" checksums.txt.sigstore.json --yes --repo turbokast/mythhelm
   ```

   A bundle is only valid when the release workflow signed it (check 3 pins that identity). When the workflow produced a replacement bundle outside the failed step, upload it over the old one with `gh release upload "$TAG" checksums.txt.sigstore.json --clobber --repo turbokast/mythhelm`.
3. Re-run the failed job from the Actions UI, with the same tag, and approve the `release` environment again. The job rebuilds and uploads the GoReleaser assets, signs `checksums.txt` again, uploads the bundle and re-attests the same subjects: `dist/checksums.txt`, `dist/*.tar.gz` and `dist/*.zip`.
4. Re-run the three verify checks above. The first real release exercises this procedure for the first time; record any difference here.
