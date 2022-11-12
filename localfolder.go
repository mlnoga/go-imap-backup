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
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// A local mail folder, consisting of an .mbox file and its corresponding index .idx
type LocalFolder struct {
	Name       string
	Mbox       *os.File
	Idx        *os.File
	IdxWriter  *bufio.Writer  // for writing to the index line by line, in append mode
	IdxScanner *bufio.Scanner // for reading the index line by line, in readonly mode
	IdxLineNo  int

	err     error       // stores mbox error
	mm      MessageMeta // message
	message []byte      // stores Text() of message
}

func GetLocalFolderNames(path string) (folderNames []string, err error) {
	dirInfos, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, dirInfo := range dirInfos {
		if dirInfo.IsDir() {
			continue
		}
		name := dirInfo.Name()
		if strings.HasSuffix(name, ".idx") {
			folderName := name[0 : len(name)-4]
			folderNames = append(folderNames, folderName)
		}
	}
	sort.Strings(folderNames)
	return folderNames, nil
}

// Open local mail folder message and index file for reading
func OpenLocalFolderReadOnly(path, folderName string) (lf *LocalFolder, err error) {
	lf = &LocalFolder{Name: folderName}

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

// Reads the entire index from a local mail folder, and returns it as folder metadata
func (lf *LocalFolder) ReadAllIndex() (f *ImapFolderMeta, err error) {
	f = &ImapFolderMeta{Name: lf.Name}
	// read line by line
	lineNo := 1
	for lf.IdxScan() {
		msg := lf.IdxText()
		f.Messages = append(f.Messages, msg)
		f.UidValidity = msg.UidValidity
		f.Size += uint64(msg.Size)
	}
	if err := lf.IdxErr(); err != nil {
		return nil, fmt.Errorf("%s:%d: %s", lf.Idx.Name(), lineNo, err.Error())
	}

	return f, nil
}

// Scan the next index file line, behaves like bufio.Scan().
func (lf *LocalFolder) IdxScan() bool {
	idxScan := lf.IdxScanner.Scan()
	lf.IdxLineNo++
	if !idxScan {
		lf.err = lf.IdxScanner.Err()
		return false
	}

	line := lf.IdxScanner.Text() // without terminating newline
	_, err := fmt.Sscanf(line, "%d\t%d\t%d\t%d", &lf.mm.UidValidity, &lf.mm.Uid, &lf.mm.Size, &lf.mm.Offset)
	if err != nil {
		lf.err = fmt.Errorf("%s:%d: %s", lf.Idx.Name(), lf.IdxLineNo, err.Error())
		return false
	}

	return true
}

// Returns error from last index file line scan, behaves like bufio.Err()
func (lf *LocalFolder) IdxErr() error {
	return lf.err
}

// Returns the MessageMeta value for the last index file line scan, behaves like bufio.Text()
func (lf *LocalFolder) IdxText() MessageMeta {
	return lf.mm
}

// Scan the next message from mbox/idx, behaves like bufio.Scan().
func (lf *LocalFolder) MboxScan() bool {
	idxScan := lf.IdxScan()
	if !idxScan {
		lf.err = lf.IdxErr()
		return false
	}
	mm := lf.IdxText()

	if _, err := lf.Mbox.Seek(int64(mm.Offset), io.SeekStart); err != nil {
		lf.err = err
		return false
	}

	if len(lf.message) < int(mm.Size) {
		lf.message = make([]byte, mm.Size)
	}

	if _, err := lf.Mbox.Read(lf.message); err != nil {
		lf.err = err
		return false
	}
	lf.err = nil
	return true
}

// Returns error from last message scan from mbox/idx, behaves like bufio.Err()
func (lf *LocalFolder) MboxErr() error {
	return lf.err
}

// Returns last message value from mbox/idx scan, behaves like bufio.Text()
func (lf *LocalFolder) MboxText() []byte {
	return lf.message
}

// Open a local mail folder for appending messages
func OpenLocalFolderAppend(path, folderName string) (lf *LocalFolder, err error) {
	// Ensure path exists
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
	pos, err := lf.Mbox.Seek(0, io.SeekCurrent)
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
