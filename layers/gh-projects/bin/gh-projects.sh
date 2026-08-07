#!/bin/sh
# The GitHub half of the gh-projects mirror.
#
#   gh-projects.sh list      print the board as [{id,title,status,url}]
#   gh-projects.sh create    create the item named by $ILK_MIRROR_TITLE, print its id
#   gh-projects.sh update    apply $ILK_MIRROR_TITLE / $ILK_MIRROR_STATUS to $ILK_MIRROR_ID
#   gh-projects.sh doctor    check the configuration and authentication
#
# ilk supplies identity, diffing, ambiguity refusal and the plan-then-apply
# discipline. Everything in this file is the part that knows what a GitHub Project
# is — kept here precisely so that ilk does not have to.
#
# This script reads its input from ILK_MIRROR_* environment variables rather than
# the JSON ilk also offers on stdin, so it needs no jq of its own. All parsing of
# GitHub's replies is done by the jq that `gh` embeds behind `--jq`.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

owner=${ILK_VAR_OWNER:-}
number=${ILK_VAR_PROJECT_NUMBER:-}
repo=${ILK_VAR_REPO:-}
status_field=${ILK_VAR_STATUS_FIELD:-Status}

die() {
	echo "gh-projects: $1" >&2
	exit 1
}

need_config() {
	command -v gh >/dev/null 2>&1 ||
		die "the GitHub CLI (gh) is not installed — see https://cli.github.com"
	[ -n "$owner" ] ||
		die "no owner configured. Set \`owner\` for this layer in .ilk/config.yaml — it is the user or organisation the project belongs to."
	[ -n "$number" ] ||
		die "no project_number configured. Set \`project_number\` for this layer in .ilk/config.yaml — it is the number in the project's URL."
}

# json_string quotes a shell value so it can be printed inside JSON.
json_string() {
	printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/^/"/' -e 's/$/"/'
}

# list prints the whole board in ilk's shape. A draft item carries its own title;
# an item backed by an issue carries the issue's, which is why both are consulted.
list() {
	need_config
	gh project item-list "$number" --owner "$owner" --limit 1000 --format json --jq '
		[ .items[]
		  | { id: .id,
		      title: (.title // .content.title // ""),
		      status: ((.status // "") | ascii_downcase),
		      url: (.content.url // "") }
		]
	'
}

create() {
	need_config
	title=${ILK_MIRROR_TITLE:-}
	path=${ILK_MIRROR_PATH:-the record}
	[ -n "$title" ] || die "the document has no title, so there is nothing to create"
	body="Tracked in the record at $path. Edit the document, not this item — \`ilk mirror apply gh-projects\` writes the change here."

	if [ -n "$repo" ]; then
		# A real issue: referenceable from commits and pull requests.
		url=$(gh issue create --repo "$repo" --title "$title" --body "$body") ||
			die "creating an issue in $repo failed. Check the repository name and that your token can write issues there."
		[ -n "$url" ] || die "creating the issue in $repo produced no url"
		id=$(gh project item-add "$number" --owner "$owner" --url "$url" --format json --jq '.id') ||
			die "created $url but could not add it to project $number. Add it by hand, or fix the project number and re-run."
		[ -n "$id" ] || die "adding $url to the project produced no item id"
		printf '{"id":%s,"url":%s}\n' "$(json_string "$id")" "$(json_string "$url")"
	else
		id=$(gh project item-create "$number" --owner "$owner" --title "$title" --body "$body" --format json --jq '.id') ||
			die "creating a draft item failed. Check that your token has the \`project\` scope."
		[ -n "$id" ] || die "creating the draft item produced no id"
		printf '{"id":%s}\n' "$(json_string "$id")"
	fi

	# The id has been printed, so ilk can record it whatever happens next. Setting
	# the status is attempted but never allowed to fail the create: an item ilk
	# cannot link to is an orphan on the board that the next run tries to create
	# again. A status that did not land shows up as a divergence in the next plan,
	# which is exactly where it can be explained properly.
	(set_status "$id" "${ILK_MIRROR_STATUS:-}") ||
		echo "gh-projects: created the item but left its status alone — run \`ilk mirror plan gh-projects\` to see why" >&2
}

update() {
	need_config
	id=${ILK_MIRROR_ID:-}
	[ -n "$id" ] || die "no item id to update"

	project_id=$(gh project view "$number" --owner "$owner" --format json --jq '.id') ||
		die "could not read project $number for $owner"
	[ -n "$project_id" ] || die "could not resolve the id of project $number"

	title=${ILK_MIRROR_TITLE:-}
	if [ -n "$title" ]; then
		# Only a draft item's title lives on the project; an issue's lives on the
		# issue and is not this mirror's to rewrite, so a rejection here is not a
		# failure of the sync.
		gh project item-edit --id "$id" --project-id "$project_id" --title "$title" >/dev/null 2>&1 || true
	fi

	set_status "$id" "${ILK_MIRROR_STATUS:-}"
}

# set_status moves the item's single-select status field.
#
# If the record names a status the board does not offer, this refuses. Inventing
# an option would put a value on somebody else's board that nobody chose, and
# quietly skipping would let the two drift while reporting success.
set_status() {
	item_id=$1
	status=$2
	[ -n "$status" ] || return 0

	project_id=${project_id:-$(gh project view "$number" --owner "$owner" --format json --jq '.id')}
	fields=$(gh project field-list "$number" --owner "$owner" --format json) ||
		die "could not list the fields of project $number"

	field_id=$(printf '%s' "$fields" | gh_filter --arg n "$status_field" '
		.fields[] | select(.name == $n) | .id
	')
	if [ -z "$field_id" ]; then
		available=$(printf '%s' "$fields" | gh_filter '[ .fields[].name ] | join(", ")')
		die "the project has no field called \"$status_field\" (it has: $available).
  fix: set \`status_field\` for this layer in .ilk/config.yaml to the field the board actually uses."
	fi

	option_id=$(printf '%s' "$fields" | gh_filter --arg n "$status_field" --arg s "$status" '
		.fields[] | select(.name == $n) | .options[]?
		| select((.name | ascii_downcase) == ($s | ascii_downcase)) | .id
	')
	if [ -z "$option_id" ]; then
		available=$(printf '%s' "$fields" | gh_filter --arg n "$status_field" '
			[ .fields[] | select(.name == $n) | .options[]?.name ] | join(", ")
		')
		die "the record says status \"$status\", which \"$status_field\" does not offer (it has: $available).
  fix: add or rename the option on the board, or change the document's status to one of those."
	fi

	gh project item-edit --id "$item_id" --project-id "$project_id" \
		--field-id "$field_id" --single-select-option-id "$option_id" >/dev/null ||
		die "could not set $status_field on item $item_id"
}

# gh_filter runs a jq program over stdin.
#
# gh embeds jq but only applies it to its own responses, so filtering a saved
# response needs a real jq. Only the status-field lookup reaches here; everything
# else is done by gh's own --jq, which is why a project without a status field
# needs no jq at all.
gh_filter() {
	command -v jq >/dev/null 2>&1 ||
		die "setting the status field needs jq to read the project's field list. Install jq (https://jqlang.github.io/jq), or drop \`status\` from the documents' frontmatter to sync titles only."
	jq -r "$@"
}

doctor() {
	command -v gh >/dev/null 2>&1 ||
		die "the GitHub CLI (gh) is not installed — see https://cli.github.com"
	gh auth status >/dev/null 2>&1 ||
		die "gh is not authenticated. Run \`gh auth login\`."
	[ -n "$owner" ] ||
		die "no \`owner\` configured for this layer in .ilk/config.yaml"
	[ -n "$number" ] ||
		die "no \`project_number\` configured for this layer in .ilk/config.yaml"

	title=$(gh project view "$number" --owner "$owner" --format json --jq '.title' 2>/dev/null) ||
		die "cannot read project $number for $owner. Check the number, the owner, and that your token has the \`project\` scope (\`gh auth refresh -s project\`)."
	[ -n "$title" ] || die "project $number for $owner reported no title, which usually means the number is wrong"
	echo "gh-projects: connected to \"$title\" ($owner/$number)"
}

case "${1:-}" in
list) list ;;
create) create ;;
update) update ;;
doctor) doctor ;;
*)
	echo "usage: gh-projects.sh list|create|update|doctor" >&2
	exit 2
	;;
esac
