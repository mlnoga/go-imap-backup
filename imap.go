// go-imap-backup (C) 2022 by Markus L. Noga
// Backup, restore and delete old messages from an IMAP server
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
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	pb "github.com/schollz/progressbar/v3"
	"io"
	"math"
	"sort"
	"time"
)

// Retrieves a list of all folders from an Imap server
func ListFolders(c *client.Client) ([]string, error) {
	// Query list of folders
	mailboxesCh := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxesCh)
	}()

	// Collect results
	mailboxes := []string{}
	for m := range mailboxesCh {
		mailboxes = append(mailboxes, m.Name)
	}
	if err := <-done; err != nil {
		return nil, err
	}

	sort.Strings(mailboxes)
	return mailboxes, nil
}

// Creates local metadata for an imap folder by fetching metadata for all its messages
func NewImapFolderMeta(c *client.Client, folderName string) (ifm *ImapFolderMeta, err error) {
	ifm = &ImapFolderMeta{Name: folderName}
	mbox, err := c.Select(folderName, true)
	if err != nil {
		return nil, err
	}
	ifm.UidValidity = mbox.UidValidity
	if mbox.Messages == 0 {
		return ifm, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)
	items := []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size}

	messages := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	ifm.Messages = []MessageMeta{}
	for msg := range messages {
		d := MessageMeta{SeqNum: msg.SeqNum, UidValidity: mbox.UidValidity, Uid: msg.Uid, Size: msg.Size, Offset: math.MaxUint64}
		ifm.Messages = append(ifm.Messages, d)
		ifm.Size += uint64(msg.Size)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	return ifm, nil
}

// Download the given set of messages from the remote Imap mailbox,
// and save them to local folders using the remote folder name,
// reporting download progress in bytes to the progress bar after every message
func (f *ImapFolderMeta) DownloadTo(c *client.Client, lf *LocalFolder, bar *pb.ProgressBar) error {
	// Select mailbox on server
	mbox, err := c.Select(f.Name, true)
	if err != nil {
		return err
	}
	if mbox.UidValidity != f.UidValidity {
		return fmt.Errorf("UidValidity changed from %d to %d, this should not happen",
			mbox.UidValidity, f.UidValidity)
	}

	// prepare sequence set and trigger download of messages
	totalSize := uint64(0)
	seqset := new(imap.SeqSet)
	for _, message := range f.Messages {
		seqset.AddNum(message.SeqNum)
		totalSize += uint64(message.Size)
	}

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size, imap.FetchEnvelope, section.FetchItem()}

	messages := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	// process messages received
	for msg := range messages {
		// print progress
		if err := bar.Add64(int64(msg.Size)); err != nil {
			return err
		}

		// read message into memory
		r := msg.GetBody(section)
		if r == nil {
			return fmt.Errorf("Server didn't return message body")
		}
		bs, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		var env string
		if len(msg.Envelope.From)>0 {
			env = msg.Envelope.From[0].Address()
		}
		date := msg.Envelope.Date
		if err := lf.Append(mbox.UidValidity, msg.Uid, env, date, bs); err != nil {
			return err
		}
	}
	if err := <-done; err != nil {
		return err
	}
	return nil
}

// Delete messages before the given time from an Imap server
func DeleteMessagesBefore(c *client.Client, folderName string, before time.Time) (numDeleted int, err error) {
	mbox, err := c.Select(folderName, false) // need r/w access
	if err != nil {
		return 0, err
	}
	if mbox.Messages == 0 {
		return 0, nil
	}

	ids, err := findMessagesBefore(c, before)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	err = deleteMessages(c, ids)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func findMessagesBefore(c *client.Client, before time.Time) ([]uint32, error) {
	criteria := imap.NewSearchCriteria()
	criteria.Before = before
	return c.Search(criteria)
}

func deleteMessages(c *client.Client, ids []uint32) error {
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}
	if err := c.Store(seqset, item, flags, nil); err != nil {
		return err
	}

	return c.Expunge(nil)
}
