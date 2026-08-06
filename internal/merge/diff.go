// Package merge implements the line-based three-way merge that lets a layer
// upgrade change a file somebody has also edited.
//
// Without it, ilk can only refuse: it knows the file no longer matches what it
// wrote, but not whether the two changes actually collide. In practice they
// usually do not — the layer rewrote one section and the user edited another —
// and refusing every such case makes upgrades painful enough that people stop
// running them.
//
// The implementation is deliberately conservative. It merges only when the two
// sides touched disjoint regions or made identical changes; anything else is
// reported as a conflict rather than resolved by guesswork. A wrong merge is far
// worse than a refusal, because a refusal is visible.
package merge

// maxCells bounds the diff's dynamic-programming table. Beyond it the merge is
// declined rather than allowed to consume unbounded memory. After common
// prefixes and suffixes are trimmed, real files land far below this.
const maxCells = 4_000_000

// match is a pair of aligned indices between two line slices.
type match struct{ a, b int }

// align returns the longest common subsequence of a and b as index pairs, or
// ok=false when the inputs are too large to align within maxCells.
//
// Trimming the shared head and tail first is what keeps this cheap: a
// one-paragraph edit in a long document reduces to a diff of a few lines.
func align(a, b []string) (matches []match, ok bool) {
	head := 0
	for head < len(a) && head < len(b) && a[head] == b[head] {
		head++
	}

	tail := 0
	for tail < len(a)-head && tail < len(b)-head &&
		a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}

	midA := a[head : len(a)-tail]
	midB := b[head : len(b)-tail]

	if (len(midA)+1)*(len(midB)+1) > maxCells {
		return nil, false
	}

	matches = make([]match, 0, head+tail+min(len(midA), len(midB)))
	for i := 0; i < head; i++ {
		matches = append(matches, match{i, i})
	}
	for _, m := range lcs(midA, midB) {
		matches = append(matches, match{m.a + head, m.b + head})
	}
	for i := 0; i < tail; i++ {
		matches = append(matches, match{len(a) - tail + i, len(b) - tail + i})
	}
	return matches, true
}

// lcs computes a longest common subsequence by the textbook dynamic program.
func lcs(a, b []string) []match {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}

	// table[i][j] is the LCS length of a[i:] and b[j:].
	table := make([][]int32, len(a)+1)
	buf := make([]int32, (len(a)+1)*(len(b)+1))
	for i := range table {
		table[i] = buf[i*(len(b)+1) : (i+1)*(len(b)+1)]
	}

	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}

	var out []match
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, match{i, j})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}

// mapping inverts a match list into a lookup from a-index to b-index, with -1
// for lines that have no counterpart.
func mapping(matches []match, size int) []int {
	out := make([]int, size)
	for i := range out {
		out[i] = -1
	}
	for _, m := range matches {
		out[m.a] = m.b
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
