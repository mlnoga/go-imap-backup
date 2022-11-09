// go-imap-backup (C) 2022 by Markus L. Noga
// Backup messages from an IMAP server, optionally deleting older messages
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"fmt"
)

// Returns a slice of all strings which are in as and bs, in stable order of as
func intersect(as []string, bs []string) []string {
	have := make(map[string]bool)
	for _, b := range bs {
		have[b] = true
	}
	cs := []string{}
	for _, a := range as {
		if _, ok := have[a]; ok {
			cs = append(cs, a)
		}
	}
	return cs
}

// Print a given size in bytes as a human-readable string
// using KB, MB, GB, TB as appropriate.
func humanReadableSize(n uint64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	} else if n < 10*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	} else if n < 1024*1024 {
		return fmt.Sprintf("%d KB", n/1024)
	} else if n < 10*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	} else if n < 1024*1024*1024 {
		return fmt.Sprintf("%d MB", n/1024/1024)
	} else if n < 10*1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(n)/1024/1024/1024)
	} else if n < 1024*1024*1024*1024 {
		return fmt.Sprintf("%d GB", n/1024/1024/1024)
	} else if n < 10*1024*1024*1024*1024 {
		return fmt.Sprintf("%.1f TB", float64(n)/1024/1024/1024/1024)
	} else {
		return fmt.Sprintf("%d TB", n/1024/1024/1024/1024)
	}
}
