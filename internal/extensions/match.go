package extensions

import "sort"

// MatchHostIDs returns the subset of extensions entry IDs for the given harness
// whose ID or HostAliases match any value in hostIDs. The returned slice is
// sorted lexicographically for stable wizard rendering.
//
// Matching rules:
//   - Case-sensitive, exact string equality.
//   - An entry matches if its ID OR any of its HostAliases is found in hostIDs.
//   - hostIDs is treated as a set; duplicates are ignored.
//   - Only entries belonging to `harness` are considered.
//
// Typical usage: wizard calls hostprobe.ProbeAll(), gets back raw host-side
// IDs, then asks extensions.MatchHostIDs to translate them into extensions entry
// IDs it can pre-check in the selection step.
func MatchHostIDs(entries map[string]*Entry, harness string, hostIDs []string) []string {
	if len(hostIDs) == 0 || len(entries) == 0 {
		return nil
	}

	// Build a set for O(1) host-id lookups.
	wanted := make(map[string]struct{}, len(hostIDs))
	for _, h := range hostIDs {
		wanted[h] = struct{}{}
	}

	var matched []string
	for _, e := range entries {
		if e.Harness != harness {
			continue
		}
		if _, ok := wanted[e.ID]; ok {
			matched = append(matched, e.ID)
			continue
		}
		for _, alias := range e.HostAliases {
			if _, ok := wanted[alias]; ok {
				matched = append(matched, e.ID)
				break
			}
		}
	}

	sort.Strings(matched)
	return matched
}
