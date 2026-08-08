#!/bin/sh
# Record and list questions that need a person.
#
#   ilk ask-human open [--decision] "Should retries live in the gateway?"
#   ilk ask-human list [--all]
#   ilk ask-human check-options
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

# The group as well as the directory: `ask.questions` is `<group>/<questions_dir>`,
# and reading only the second half sent every verb looking in `questions/` at the
# repository root, which does not exist. Nothing caught it until a check assertion
# planted a fixture where the capability actually points.
dir=${ILK_VAR_GROUP:-docs}/${ILK_VAR_QUESTIONS_DIR:-questions}
mode=${1:-}
# Not `[ $# -gt 0 ] && shift`: under `set -e` that exits 1 when there are no
# arguments, which is the one case that should print the usage.
if [ $# -gt 0 ]; then
	shift
fi

case "$mode" in
open)
	kind=question
	if [ "${1:-}" = --decision ]; then
		kind=decision
		shift
	fi
	title=$*
	if [ -z "$title" ]; then
		echo 'usage: ilk ask-human open [--decision] "the question"' >&2
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
kind: $kind
asked: $today
EOF

	# Written empty on purpose, so `ilk check` fails until they are filled in. A
	# scaffold with plausible-looking placeholder consequences would pass the check
	# and answer nothing, which is the worse failure of the two.
	if [ "$kind" = decision ]; then
		cat >>"$file" <<'EOF'
options:
  - id: first
    label:
    consequence:
  - id: second
    label:
    consequence:
recommended:
EOF
	fi

	cat >>"$file" <<EOF
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
	if [ "$kind" = decision ]; then
		echo "Fill in the options — a label and, for each, what picking it would mean."
		echo "Then recommend one and say why in \"What I would do without an answer\"."
	else
		echo "Fill in what you would do without an answer — that is what makes this cheap to answer."
	fi
	echo "Then carry on with everything the answer does not block."
	;;

check-options)
	# Only open decisions. An answered one is history, and a question written
	# before this layer knew about options is not a defect anybody can repair.
	#
	# A function rather than an inline command substitution: bash 3.2 — which is
	# /bin/sh on macOS — cannot parse a `case` pattern's `)` inside `$( )`, and
	# reports it as a syntax error at a line nowhere near the cause.
	options_report() {
		[ -d "$dir" ] || return 0
		find "$dir" -type f -name '*.md' | LC_ALL=C sort | while IFS= read -r file; do
			case "${file##*/}" in README.md) continue ;; esac
			awk -v f="$file" '
			function val(s) {
				sub(/^[^:]*:[ \t]*/, "", s)
				sub(/[ \t]+$/, "", s)
				gsub(/^"|"$/, "", s)
				return s
			}
			NR == 1 { if ($0 ~ /^---[ \t]*$/) { fm = 1; next } else { exit } }
			!fm { next }
			$0 ~ /^---[ \t]*$/ { exit }
			/^options:[ \t]*$/ { inopt = 1; next }
			/^[A-Za-z_]/ {
				inopt = 0
				if ($0 ~ /^kind:/) kind = val($0)
				if ($0 ~ /^status:/) status = val($0)
				if ($0 ~ /^recommended:/) rec = val($0)
				next
			}
			inopt && /^[ \t]*-[ \t]+id:/ { n++; ids[n] = val($0); next }
			inopt && /^[ \t]*label:/ { if (n > 0) labels[n] = val($0); next }
			inopt && /^[ \t]*consequence:/ { if (n > 0) cons[n] = val($0); next }
			END {
				if (kind != "decision" || status != "open") exit
				if (n < 2) {
					printf "%s: `kind: decision` offering %d option(s) — a decision needs at least two things to choose between\n", f, n
					exit
				}
				for (i = 1; i <= n; i++) {
					if (labels[i] == "")
						printf "%s: option `%s` has no `label:`\n", f, ids[i]
					if (cons[i] == "")
						printf "%s: option `%s` says nothing about what picking it would mean\n", f, ids[i]
				}
				if (rec == "") {
					printf "%s: no `recommended:` — say which one you would pick\n", f
					exit
				}
				for (i = 1; i <= n; i++) if (ids[i] == rec) exit
				printf "%s: `recommended: %s` is not one of the options offered\n", f, rec
			}
			' "$file"
		done
	}
	report=$(options_report)
	[ -n "$report" ] || exit 0
	printf '%s\n' "$report"
	exit 1
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
			if [ "$want_blocking" = "true" ]; then
				marker="! "
			fi
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
usage: ilk ask-human open "the question"           record a question that needs a person
       ilk ask-human open --decision "..."         record one that offers options
       ilk ask-human list [--all]                  show open questions, blocking first
EOF
	exit 2
	;;
esac
