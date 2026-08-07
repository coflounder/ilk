#!/bin/sh
# Record and list questions that need a person.
#
#   ilk ask-human open "Should retries live in the gateway?"
#   ilk ask-human list [--all]
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

dir=${ILK_VAR_QUESTIONS_DIR:-questions}
mode=${1:-}
[ $# -gt 0 ] && shift

case "$mode" in
open)
	title=$*
	if [ -z "$title" ]; then
		echo 'usage: ilk ask-human open "the question"' >&2
		exit 2
	fi

	slug=$(printf '%s' "$title" |
		tr '[:upper:]' '[:lower:]' |
		sed -e 's/[^a-z0-9]\{1,\}/-/g' -e 's/^-//' -e 's/-$//' |
		cut -c1-60)
	[ -n "$slug" ] || { echo "ilk ask-human: the question has no usable characters" >&2; exit 1; }

	[ -d "$dir" ] || { echo "ilk ask-human: no $dir/ directory — run \`ilk apply\` first" >&2; exit 1; }

	today=$(date +%Y-%m-%d)
	file="$dir/Q-$slug.md"
	[ ! -e "$file" ] || { echo "ilk ask-human: $file already exists" >&2; exit 1; }

	cat >"$file" <<EOF
---
id: q-$slug
title: $title
status: open
blocking: true
asked: $today
---

# $title

## Why this needs a person

<!-- What makes this not yours to decide: expensive to reverse, requirements that
     contradict each other, access or authority you do not have. -->

## What I would do without an answer

<!-- Your best guess, and what it would cost if it turned out wrong. This is the
     most useful part of the question: it lets somebody answer with "yes, do that"
     in four seconds instead of writing an essay. -->

## What is blocked

<!-- Name it. If the honest answer is "nothing, I can work around this", set
     blocking: false above — marking everything blocking is how the signal stops
     meaning anything. -->

## Answer

<!-- Filled in by whoever answers. Set status: answered when it is. -->
EOF

	echo "$file"
	echo
	echo "Fill in what you would do without an answer — that is what makes this cheap to answer."
	echo "Then carry on with everything the answer does not block."
	;;

list)
	[ -d "$dir" ] || { echo "ilk ask-human: no $dir/ directory" >&2; exit 1; }

	show_all=0
	for arg in "$@"; do
		case "$arg" in
		--all) show_all=1 ;;
		*) echo "ilk ask-human list: unknown argument $arg" >&2; exit 2 ;;
		esac
	done

	found=0
	# Blocking first: those are the ones stopping work.
	for want_blocking in true false; do
		for file in "$dir"/*.md; do
			[ -e "$file" ] || continue
			case "$(basename "$file")" in README.md) continue ;; esac

			status=$(sed -n 's/^status:[[:space:]]*//p' "$file" | head -n 1 | tr -d "\"' ")
			blocking=$(sed -n 's/^blocking:[[:space:]]*//p' "$file" | head -n 1 | tr -d "\"' ")
			title=$(sed -n 's/^title:[[:space:]]*//p' "$file" | head -n 1 | sed -e 's/^"//' -e 's/"$//')
			asked=$(sed -n 's/^asked:[[:space:]]*//p' "$file" | head -n 1 | tr -d "\"' ")

			[ "$blocking" = "$want_blocking" ] || continue
			if [ "$show_all" -eq 0 ] && [ "$status" != "open" ]; then
				continue
			fi

			marker="  "
			[ "$want_blocking" = "true" ] && marker="! "
			printf '%s%-9s %-10s %s\n' "$marker" "$status" "$asked" "$title"
			printf '            %s\n' "$file"
			found=$((found + 1))
		done
	done

	if [ "$found" -eq 0 ]; then
		if [ "$show_all" -eq 0 ]; then
			echo "No open questions."
		else
			echo "No questions recorded."
		fi
		exit 0
	fi
	echo
	echo "! marks a blocking question — work is stopped until it is answered."
	;;

*)
	cat >&2 <<'EOF'
usage: ilk ask-human open "the question"    record a question that needs a person
       ilk ask-human list [--all]           show open questions, blocking first
EOF
	exit 2
	;;
esac
