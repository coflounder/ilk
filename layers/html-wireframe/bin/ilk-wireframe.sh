#!/bin/sh
# Wireframes: what an interface was agreed to look like, before it was built.
#
#   ilk-wireframe.sh new "Checkout — payment step"
#   ilk-wireframe.sh check-self-contained
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

dir="${ILK_VAR_GROUP:-docs}/${ILK_VAR_WIREFRAMES_DIR:-wireframes}"
mode=${1:-}
[ $# -gt 0 ] && shift

case "$mode" in
new)
	title=$*
	if [ -z "$title" ]; then
		echo 'usage: ilk html-wireframe new "the screen"' >&2
		exit 2
	fi

	slug=$(printf '%s' "$title" |
		tr '[:upper:]' '[:lower:]' |
		sed -e 's/[^a-z0-9]\{1,\}/-/g' -e 's/^-//' -e 's/-$//' |
		cut -c1-60)
	[ -n "$slug" ] || { echo "ilk html-wireframe: the title has no usable characters" >&2; exit 1; }
	[ -d "$dir" ] || { echo "ilk html-wireframe: no $dir/ directory — run \`ilk apply\` first" >&2; exit 1; }

	file="$dir/WF-$slug.html"
	[ ! -e "$file" ] || { echo "ilk html-wireframe: $file already exists" >&2; exit 1; }

	# Greyscale, one system font, boxes and labels. The fidelity is the message:
	# a sketch that looks finished gets reactions about the shade of the button,
	# and a sketch that looks like a sketch gets reactions about the flow.
	cat >"$file" <<EOF
<!doctype html>
<meta charset="utf-8">
<title>$title — wireframe</title>
<style>
  /* Deliberately plain. If you find yourself reaching for a colour, the
     question you want answered is probably not one a wireframe can answer. */
  body { font: 15px/1.5 system-ui, sans-serif; color: #222; margin: 2rem; max-width: 62rem; }
  h1 { font-size: 1.1rem; font-weight: 600; margin: 0 0 .25rem; }
  .meta { color: #777; margin: 0 0 2rem; font-size: .85rem; }
  .frame { border: 1px solid #999; padding: 1rem; margin: 0 0 1rem; }
  .frame > .label { color: #777; font-size: .75rem; text-transform: uppercase; letter-spacing: .06em; }
  .box { border: 1px solid #bbb; background: #f4f4f4; padding: .75rem; margin: .5rem 0; }
  .box.optional { border-style: dashed; background: none; }
  .row { display: flex; gap: .75rem; }
  .row > * { flex: 1; }
  .note { border-left: 3px solid #ccc; padding-left: .75rem; color: #555; margin: .5rem 0 1.5rem; }
  .btn { display: inline-block; border: 1px solid #666; padding: .4rem .9rem; }
</style>

<h1>$title</h1>
<p class="meta">Wireframe — layout and flow only. Not colour, not copy, not final.</p>

<div class="frame">
  <div class="label">Replace this with the screen</div>
  <div class="box">Header</div>
  <div class="row">
    <div class="box">Left</div>
    <div class="box optional">Optional, dashed</div>
  </div>
  <div class="box"><span class="btn">Primary action</span></div>
</div>

<p class="note">
  <strong>What this is asking:</strong> say what you want disagreed with. A wireframe
  with no question attached gets "looks good" and settles nothing.
</p>

<div class="frame">
  <div class="label">Empty state</div>
  <div class="box optional">What is here before there is anything</div>
</div>

<div class="frame">
  <div class="label">Error state</div>
  <div class="box optional">What is here when it goes wrong, and where the message sits</div>
</div>
EOF

	echo "$file"
	echo
	echo "Link it from the spec's \`## Wireframe\` section:"
	echo
	# Relative to the spec, not to the repository root: links are resolved from
	# the document they are written in, and a root-relative path here would be a
	# broken link that `wireframe.exists` then reports as a wireframe nobody drew.
	echo "    - [$title](../${ILK_VAR_WIREFRAMES_DIR:-wireframes}/WF-$slug.html)"
	echo
	echo "Draw the empty and error states too — they are where the disagreements are."
	;;

check-self-contained)
	report=$(
		[ -d "$dir" ] || exit 0
		find "$dir" -type f -name '*.html' | LC_ALL=C sort | while IFS= read -r file; do
			if grep -qi '<script' "$file"; then
				echo "$file: has a <script> — a wireframe that runs is a prototype, and people react to a prototype as though it were the product"
			fi
			# Only resource loads. An `<a href="https://...">` is a link nobody
			# clicks in a sketch; a stylesheet or an image is the thing that
			# renders as a blank box on the day somebody opens this offline.
			if grep -qiE 'src="https?://|<link[^>]+href="https?://' "$file"; then
				echo "$file: loads something over the network, so it renders differently — or not at all — for whoever opens it next"
			fi
		done
	)
	[ -n "$report" ] || exit 0
	printf '%s\n' "$report"
	exit 1
	;;

*)
	cat >&2 <<'EOF'
usage: ilk html-wireframe new "the screen"    start a wireframe
EOF
	exit 2
	;;
esac
