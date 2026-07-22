#!/usr/bin/env sh
#
# Publish "Home Assistant Plugin/" to the public GitHub repo HACS installs from.
#
# HACS requires custom_components/<domain>/ at the root of the repository, so
# the plugin cannot be installed from this monorepo directly. This splits the
# folder out into its own commit history — where custom_components/gsmnode/ is
# at the top level — and pushes that to GitHub.
#
# The monorepo stays the source of truth. Nothing here is a fork: the split is
# recomputed from scratch every run, so the public repo is always exactly what
# this folder contains.
#
# First run, once:
#   git remote add github https://github.com/freefall-dev/gsmnode-ha.git
#
# Then, from the repo root:
#   sh scripts/publish-ha-plugin.sh
#
set -eu

PREFIX="Home Assistant Plugin"
REMOTE="${1:-github}"
BRANCH="${2:-main}"

cd "$(dirname "$0")/.."

if ! git remote get-url "$REMOTE" >/dev/null 2>&1; then
	echo "No '$REMOTE' remote. Add it first:" >&2
	echo "  git remote add $REMOTE https://github.com/freefall-dev/gsmnode-ha.git" >&2
	exit 1
fi

# A dirty tree would publish something that is not in any commit.
if ! git diff-index --quiet HEAD -- "$PREFIX"; then
	echo "'$PREFIX' has uncommitted changes. Commit them first." >&2
	exit 1
fi

echo "Splitting '$PREFIX' …"
split_ref=$(git subtree split --prefix="$PREFIX")

echo "Pushing $split_ref -> $REMOTE/$BRANCH"
git push "$REMOTE" "$split_ref:refs/heads/$BRANCH"

echo
echo "Done. To cut a release HACS will offer (tag must exist on GitHub, and a"
echo "GitHub *release* — not just a tag — is what HACS reads):"
echo "  git push $REMOTE $split_ref:refs/heads/$BRANCH"
echo "  gh release create v$(sed -n 's/.*\"version\": \"\([^\"]*\)\".*/\1/p' \
	"$PREFIX/custom_components/gsmnode/manifest.json") \\"
echo "      --repo freefall-dev/gsmnode-ha --generate-notes"
