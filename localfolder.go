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
	"bufio"
	"fmt"
	"os"
	"time"
)

// A local mail folder, consisting of an .mbox file and its corresponding index .idx
type LocalFolder struct {
	Mbox       *os.File
	Idx        *os.File
	IdxWriter  *bufio.Writer  // for writing to the index line by line, in append mode
	IdxScanner *bufio.Scanner // for reading the index line by line, in readonly mode
	IdxLineNo  int
}

// Open local mail folder message and index file for reading
func OpenLocalFolderReadOnly(server, user, folderName string) (lf *LocalFolder, err error) {
	lf = &LocalFolder{}
	path := server + "/" + user

	// open mailbox file readonly
	lf.Mbox, err = os.Open(path + "/" + folderName + ".mbox")
	if err != nil {
		return nil, err
	}

	// open index file readonly
	lf.Idx, err = os.Open(path + "/" + folderName + ".idx")
	if err != nil {
		lf.Mbox.Close()
		return nil, err
	}
	lf.IdxScanner = bufio.NewScanner(lf.Idx)
	lf.IdxLineNo = 1

	return lf, nil
}

// Reads the entire index from a local mail folder, and returns it as a hash map
func (lf *LocalFolder) ReadAllIndex() (res map[uint64]MessageMeta, uidValidity uint32, err error) {
	// read line by line
	res = make(map[uint64]MessageMeta)
	lineNo := 1
	for lf.IdxScan() {
		mm, err := lf.IdxText()
		if err != nil {
			return nil, 0, err
		}
		uidValidity = mm.UidValidity
		res[mm.GetUuid()] = mm
	}
	if err := lf.IdxErr(); err != nil {
		return nil, 0, fmt.Errorf("%s:%d: %s", lf.Idx.Name(), lineNo, err.Error())
	}

	return res, uidValidity, nil
}

// Scan the next index file line, behaves like bufio.Scan().
func (lf *LocalFolder) IdxScan() bool {
	res := lf.IdxScanner.Scan()
	lf.IdxLineNo++
	return res
}

// Returns error from last index file line scan, behaves like bufio.Err()
func (lf *LocalFolder) IdxErr() error {
	return lf.IdxScanner.Err()
}

// Parse the current index line into a MessageMeta value, behaves like bufio.Text()
func (lf *LocalFolder) IdxText() (mm MessageMeta, err error) {
	line := lf.IdxScanner.Text() // without terminating newline
	_, err = fmt.Sscanf(line, "%d\t%d\t%d\t%d", &mm.UidValidity, &mm.Uid, &mm.Size, &mm.Offset)
	if err != nil {
		return mm, fmt.Errorf("%s:%d: %s", lf.Idx.Name(), lf.IdxLineNo, err.Error())
	}
	return mm, nil
}

// Open a local mail folder for appending messages
func OpenLocalFolderAppend(server, user, folderName string) (lf *LocalFolder, err error) {
	// Ensure path exists
	path := server + "/" + user
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}

	lf = &LocalFolder{}
	// open mailbox file for appending
	mboxName := path + "/" + folderName + ".mbox"
	lf.Mbox, err = os.OpenFile(mboxName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0400)
	if err != nil {
		return nil, err
	}

	// open mailbox index file for appending
	idxName := path + "/" + folderName + ".idx"
	lf.Idx, err = os.OpenFile(idxName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0400)
	if err != nil {
		lf.Mbox.Close()
		return nil, err
	}
	lf.IdxWriter = bufio.NewWriter(lf.Idx)
	return lf, nil
}

// Appends a message to a local mail folder
func (lf *LocalFolder) Append(uidValidity, uid uint32, from string, when time.Time, bs []byte) error {
	// write header into mbox file
	header := fmt.Sprintf("From %s %s\n", from, when.UTC().Format(time.ANSIC))
	_, err := fmt.Fprintf(lf.Mbox, "%s", header)
	if err != nil {
		return err
	}

	// retrieve current mbox file size in bytes, for storing in index file
	pos, err := lf.Mbox.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}

	// write message body into mbox file
	_, err = lf.Mbox.Write(bs)
	if err != nil {
		return err
	}

	// write separating blank line into mbox file
	_, err = fmt.Fprintf(lf.Mbox, "\n")
	if err != nil {
		return err
	}

	// write corresponding index record to idx file
	fmt.Fprintf(lf.IdxWriter, "%d\t%d\t%d\t%d\n", uidValidity, uid, len(bs), pos)
	return nil
}

// Close a local mail folder
func (lf *LocalFolder) Close() {
	lf.Mbox.Close()
	lf.Mbox = nil
	if lf.IdxWriter != nil {
		lf.IdxWriter.Flush()
		lf.IdxWriter = nil
	}
	lf.IdxScanner = nil
	lf.Idx.Close()
	lf.Idx = nil
}
