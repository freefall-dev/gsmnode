# Publishing

How this folder becomes `github.com/freefall-dev/gsmnode-ha`, and how that
repository gets into HACS.

HACS installs from a repository whose **root** holds
`custom_components/<domain>/`. gsmnode is a monorepo — six surfaces, of which
this is one — so the integration is split out into a repository of its own
rather than moved. The monorepo stays the source of truth.

## One-time GitHub setup

Everything on this list is checked by HACS, either by its validation action or
by the review on the `hacs/default` pull request.

1. Create **`freefall-dev/gsmnode-ha`**, public. Ideally with nothing added — no
   README, no `.gitignore`, no license — because this folder supplies all three.

   If GitHub's "Add a license" or "Add a README" box was ticked, the repository
   already has an `Initial commit` that shares no history with the split, and
   the first push is rejected as non-fast-forward. Replace it once:

   ```sh
   git fetch github-gsmnode-ha main                        # look at what is there first
   git subtree split --prefix="Home Assistant Plugin" \
     | xargs -I{} git push --force github-gsmnode-ha {}:refs/heads/main
   ```

   Only ever for that first push, and only after confirming the commit being
   discarded is GitHub's generated one and nothing else.
2. Set the repository **description**. HACS shows it in the store, and a missing
   one fails the review. Suggested:
   *"Home Assistant integration for gsmnode — send SMS, MMS and calls through
   Android phones, and receive them back as events."*
3. Add **topics**. These are what people search the store by:
   `home-assistant`, `homeassistant`, `hacs`, `custom-component`,
   `integration`, `sms`, `mms`, `gsm`, `sms-gateway`, `android`.
4. Leave **Issues enabled** — the manifest's `issue_tracker` points at them, and
   the review checks they exist.

## Publish

From the monorepo root, once:

```sh
git remote add github-gsmnode-ha https://github.com/freefall-dev/gsmnode-ha.git
```

Then, whenever this folder changes and is committed:

```sh
sh scripts/publish-ha-plugin.sh
```

That recomputes the split from scratch and pushes it, so the public repository
is always exactly what this folder contains. Because the split is deterministic,
repeated runs fast-forward rather than conflict — *unless* commits were made on
GitHub directly (a merged pull request, say), which have no counterpart here.
Merge those into the monorepo first; otherwise the push is rejected, and forcing
it would drop them.

## Cut a release

HACS reads **releases**, not tags — a tag on its own is invisible to it, and the
`hacs/default` review requires at least one release to exist.

1. Bump `version` in `custom_components/gsmnode/manifest.json`.
2. Commit, then `sh scripts/publish-ha-plugin.sh`.
3. Tag the pushed commit on GitHub and publish a release from it:

   ```sh
   gh release create v3.3.0 --repo freefall-dev/gsmnode-ha --generate-notes
   ```

   Keep the tag and the manifest `version` in step; HACS shows the tag as the
   version and offers the five most recent.

## Get it into the HACS default store

Both workflows in `.github/workflows/validate.yml` must be green **with no
errors and no ignores** — a single ignored check is grounds for the pull request
to be turned down.

1. Push, and confirm **HACS** and **hassfest** both pass on GitHub Actions.
2. Publish at least one release (above).
3. Open a pull request against
   [`hacs/default`](https://github.com/hacs/default) adding
   `freefall-dev/gsmnode-ha` to the `integration` file, **in alphabetical
   order** — the review checks the sorting.
4. The automated review then verifies the description, topics, issues, release,
   brand assets and manifest. Brands are already satisfied by
   `custom_components/gsmnode/brand/icon.png`, so nothing needs submitting to
   [home-assistant/brands](https://github.com/home-assistant/brands).

Only the repository owner or a major contributor may submit, and the
`codeowners` entry in the manifest is what the review matches against.

Until that pull request merges, the integration installs fine as a **custom
repository** — see the README.
