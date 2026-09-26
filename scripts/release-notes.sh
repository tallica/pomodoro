#!/bin/sh
# Print a tag's release notes: its CHANGELOG.md section, then install notes.
#
# Usage: scripts/release-notes.sh v0.3.0
#
# The release workflow publishes this as the release body, so a tag pushed
# without a CHANGELOG entry fails the release rather than publishing an empty
# one.
set -eu

tag=${1:-}
if [ -z "$tag" ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi

version=${tag#v}

notes=$(awk -v version="$version" '
	index($0, "## [" version "]") == 1 { found = 1; next }
	found && (index($0, "## ") == 1 || /^\[[^]]+\]: /) { exit }
	found { lines[++count] = $0 }
	END {
		# Trim the blank lines the section boundaries leave at either end.
		first = 1
		while (first <= count && lines[first] == "") first++
		while (count >= first && lines[count] == "") count--
		for (i = first; i <= count; i++) print lines[i]
	}
' CHANGELOG.md)

if [ -z "$notes" ]; then
	echo "$0: no '## [$version]' section in CHANGELOG.md" >&2
	exit 1
fi

printf '%s\n' "$notes"
cat <<'NOTES'

## Install

The attached app is ad-hoc signed, not notarized, so macOS Gatekeeper will
flag it as from an unidentified developer on first launch. Right-click the
app in Finder and choose **Open** (then confirm) to bypass this — only
needed once.

See [CHANGELOG.md](https://github.com/tallica/pomodoro/blob/master/CHANGELOG.md) for details.
NOTES
