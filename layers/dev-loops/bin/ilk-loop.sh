#!/bin/sh
# Run a command repeatedly until a gate passes.
#
#   ilk dev-loops run '<command>' [--gate '<command>'] [--max N]
#
# Two properties make this safe rather than reckless. State lives in the repository,
# so progress survives any single attempt ending. And completion is decided by the
# gate — something that cannot lie — never by the model's own assessment of its work.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

# ilk parses the flags and hands them over as ILK_FLAG_*; an empty one means it
# was not given, so the layer's variable applies.
gate=${ILK_FLAG_GATE:-${ILK_VAR_GATE:-ilk check}}
max=${ILK_FLAG_MAX:-${ILK_VAR_MAX_ITERATIONS:-10}}
log_dir=${ILK_VAR_LOG_DIR:-scratch/loops}
command=${1:-}

if [ -z "$command" ]; then
	echo "ilk dev-loops run: nothing to run" >&2
	echo "  usage: ilk dev-loops run '<command>' [--gate '<command>'] [--max N]" >&2
	exit 2
fi

case "$max" in
'' | *[!0-9]*)
	echo "ilk dev-loops run: the iteration limit must be a number, got $max" >&2
	exit 2
	;;
esac

mkdir -p "$log_dir"
run_id=$(date +%Y%m%d-%H%M%S)

echo "gate:    $gate"
echo "command: $command"
echo "ceiling: $max attempts"
echo

# Check the gate before doing anything. Work that is already done should not be
# redone, and a gate that passes immediately usually means the task was misread.
if sh -c "$gate" >"$log_dir/$run_id-gate-0.log" 2>&1; then
	echo "The gate already passes. Nothing to do."
	echo
	echo "If that is a surprise, the gate is probably not measuring what you think:"
	echo "  $gate"
	exit 0
fi

i=1
while [ "$i" -le "$max" ]; do
	echo "── attempt $i of $max ─────────────────────────────"
	log="$log_dir/$run_id-attempt-$i.log"

	if sh -c "$command" 2>&1 | tee "$log"; then
		:
	else
		echo "  (the command exited non-zero; continuing to the gate anyway)"
	fi

	if sh -c "$gate" >"$log_dir/$run_id-gate-$i.log" 2>&1; then
		echo
		echo "Gate passed after $i attempt(s)."
		echo "Logs: $log_dir/$run_id-*"
		echo
		echo "Read the diff before trusting it. A loop converges on the gate, which is"
		echo "not the same as converging on what you wanted."
		exit 0
	fi

	echo "  gate still failing; see $log_dir/$run_id-gate-$i.log"
	i=$((i + 1))
done

echo
echo "Stopped after $max attempts without passing the gate."
echo "Logs: $log_dir/$run_id-*"
echo
echo "This is a finding, not a failure to try harder. Usually one of:"
echo "  - the gate cannot be satisfied by the change being attempted"
echo "  - the task needs a decision rather than another attempt (ilk ask-human open ...)"
echo "  - the work is not convergent, and looping was the wrong tool"
exit 1
