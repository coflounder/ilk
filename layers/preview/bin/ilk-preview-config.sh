#!/bin/sh
# Find this repository's Wrangler configuration and flatten it.
#
#   ilk-preview-config.sh path    print the configuration file's path
#   ilk-preview-config.sh flat    print every value in it as `path<TAB>value`
#
# Flat output is one line per value, keyed by its path from the top of the file:
#
#   name                                    healthy-bitz
#   workers_dev                             false
#   vars.ACCESS_AUD_PREVIEW                 0000...
#   durable_objects.bindings[1].class_name  QuestBook
#
# Objects and arrays get a line of their own, valued <object> or <array>, so the
# presence of an empty `vars: {}` is visible too. Every preview check and command
# reads this instead of parsing the file itself, so there is one parser to fix.
#
# wrangler.jsonc and wrangler.json are read in full, comments and trailing
# commas included. wrangler.toml is read minimally: tables, arrays of tables,
# `key = value`, and inline arrays of flat inline tables. Anything subtler in a
# TOML file is skipped rather than guessed at.
#
# Exit: 0 found, 1 no configuration file, 2 a usage mistake.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

find_config() {
	want=${ILK_VAR_WRANGLER_CONFIG:-auto}
	if [ "$want" != auto ] && [ -n "$want" ]; then
		if [ -f "$want" ]; then
			echo "$want"
			return 0
		fi
		echo "preview.wrangler_config names $want, which does not exist" >&2
		return 1
	fi
	for f in wrangler.jsonc wrangler.json wrangler.toml; do
		if [ -f "$f" ]; then
			echo "$f"
			return 0
		fi
	done
	echo "no wrangler.jsonc, wrangler.json or wrangler.toml at the repository root (set preview.wrangler_config if it lives elsewhere)" >&2
	return 1
}

flatten_json() {
	awk '
	{ buf = buf $0 "\n" }
	END {
		n = length(buf); i = 1; depth = 0
		while (i <= n) {
			c = substr(buf, i, 1)
			if (c == "/" && substr(buf, i + 1, 1) == "/") {
				while (i <= n && substr(buf, i, 1) != "\n") i++
				continue
			}
			if (c == "/" && substr(buf, i + 1, 1) == "*") {
				i += 2
				while (i <= n && substr(buf, i, 2) != "*/") i++
				i += 2
				continue
			}
			if (index(" \t\r\n,:", c) > 0) { i++; continue }
			if (c == "\"") {
				s = readstr()
				if (depth > 0 && kind[depth] == "o" && !haskey[depth]) {
					key[depth] = s; haskey[depth] = 1
					continue
				}
				emit(here(), s); advance()
				continue
			}
			if (c == "{" || c == "[") {
				p = here()
				emit(p, c == "{" ? "<object>" : "<array>")
				depth++
				kind[depth] = (c == "{") ? "o" : "a"
				base[depth] = p; idx[depth] = 0; haskey[depth] = 0
				i++
				continue
			}
			if (c == "}" || c == "]") {
				if (depth > 0) depth--
				advance()
				i++
				continue
			}
			j = i
			while (j <= n && index(" \t\r\n,:]}/", substr(buf, j, 1)) == 0) j++
			emit(here(), substr(buf, i, j - i)); advance()
			i = j
		}
	}
	function readstr(   s, ch) {
		s = ""; i++
		while (i <= n) {
			ch = substr(buf, i, 1)
			if (ch == "\\") { s = s substr(buf, i + 1, 1); i += 2; continue }
			if (ch == "\"") { i++; return s }
			s = s ch; i++
		}
		return s
	}
	function here() {
		if (depth == 0) return ""
		if (kind[depth] == "o") return base[depth] == "" ? key[depth] : base[depth] "." key[depth]
		return base[depth] "[" idx[depth] "]"
	}
	function advance() {
		if (depth == 0) return
		if (kind[depth] == "o") haskey[depth] = 0
		else idx[depth]++
	}
	function emit(p, v) { if (p != "") print p "\t" v }
	' "$1"
}

flatten_toml() {
	awk '
	BEGIN { q = sprintf("%c", 39); prefix = ""; pending = "" }
	function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t\r]+$/, "", s); return s }
	function unq(s) { gsub(/"/, "", s); gsub(q, "", s); gsub(/[ \t]*\.[ \t]*/, ".", s); return s }
	# Drop a comment that is not inside a string.
	function nocomment(line,   out, j, ch, in_s) {
		out = ""; in_s = ""
		for (j = 1; j <= length(line); j++) {
			ch = substr(line, j, 1)
			if (in_s == "" && ch == "#") break
			if (in_s == "" && (ch == "\"" || ch == q)) in_s = ch
			else if (in_s != "" && ch == in_s) in_s = ""
			out = out ch
		}
		return out
	}
	# How far a fragment opens brackets, outside strings.
	function balance(s,   j, ch, in_s, b) {
		b = 0; in_s = ""
		for (j = 1; j <= length(s); j++) {
			ch = substr(s, j, 1)
			if (in_s == "" && (ch == "\"" || ch == q)) in_s = ch
			else if (in_s != "" && ch == in_s) in_s = ""
			else if (in_s == "" && (ch == "[" || ch == "{")) b++
			else if (in_s == "" && (ch == "]" || ch == "}")) b--
		}
		return b
	}
	function scalar(v,   qc, e) {
		v = trim(v)
		qc = substr(v, 1, 1)
		if (qc == "\"" || qc == q) {
			v = substr(v, 2)
			e = index(v, qc)
			if (e > 0) v = substr(v, 1, e - 1)
		}
		return v
	}
	# key = value pairs of one flat inline table, without its braces.
	function pairs(p, body,   parts, k, m, eq) {
		m = split(body, parts, ",")
		for (k = 1; k <= m; k++) {
			eq = index(parts[k], "=")
			if (eq == 0) continue
			print p "." unq(trim(substr(parts[k], 1, eq - 1))) "\t" scalar(substr(parts[k], eq + 1))
		}
	}
	function value(p, v,   rest, k, op, cl) {
		v = trim(v)
		if (substr(v, 1, 1) == "[") {
			print p "\t<array>"
			rest = v; k = 0
			while ((op = index(rest, "{")) > 0) {
				cl = index(substr(rest, op), "}")
				if (cl == 0) break
				print p "[" k "]\t<object>"
				pairs(p "[" k "]", substr(rest, op + 1, cl - 2))
				rest = substr(rest, op + cl)
				k++
			}
			return
		}
		if (substr(v, 1, 1) == "{") {
			print p "\t<object>"
			pairs(p, substr(v, 2, length(v) - 2))
			return
		}
		print p "\t" scalar(v)
	}
	{
		line = trim(nocomment($0))
		if (pending != "") {
			pendval = pendval " " line
			if (balance(pendval) <= 0) { value(pending, pendval); pending = "" }
			next
		}
		if (line == "") next
		if (line ~ /^\[\[.*\]\]$/) {
			name = unq(trim(substr(line, 3, length(line) - 4)))
			if (!(name in count)) { count[name] = 0; print name "\t<array>" }
			prefix = name "[" count[name] "]"
			count[name]++
			print prefix "\t<object>"
			next
		}
		if (line ~ /^\[.*\]$/) {
			prefix = unq(trim(substr(line, 2, length(line) - 2)))
			print prefix "\t<object>"
			next
		}
		eq = index(line, "=")
		if (eq == 0) next
		k = unq(trim(substr(line, 1, eq - 1)))
		p = (prefix == "") ? k : prefix "." k
		v = trim(substr(line, eq + 1))
		if (balance(v) > 0) { pending = p; pendval = v; next }
		value(p, v)
	}
	' "$1"
}

case ${1:-} in
path)
	find_config
	;;
flat)
	cfg=$(find_config) || exit 1
	case $cfg in
	*.toml) flatten_toml "$cfg" ;;
	*) flatten_json "$cfg" ;;
	esac
	;;
*)
	echo "usage: ilk-preview-config.sh path|flat" >&2
	exit 2
	;;
esac
