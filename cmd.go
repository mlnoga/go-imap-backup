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
	"bufio"
	"bytes"
	"fmt"
	"github.com/emersion/go-imap/client"
	pb "github.com/schollz/progressbar/v3"
	"log"
	"os"
	"strings"
	"time"
)

// Queries an IMAP account for the contents of all folders with given names,
// filtering out messages already in the coresponding local storage.
func cmdQuery(c *client.Client, folderNames []string) (folders []*ImapFolderMeta, filteredMsgs int, filteredSize uint64) {
	// Process all folders
	bar := pb.Default(int64(len(folderNames)), "List")
	folders = make([]*ImapFolderMeta, len(folderNames))
	totalMsgs, totalSize := 0, uint64(0)
	for i, folderName := range folderNames {
		bar.Describe("List " + folderName)

		// Fetch metadata for all messages in the folder
		var err error
		folders[i], err = NewImapFolderMeta(c, folderName)
		if err != nil {
			log.Fatal(err)
		}
		f := folders[i]
		totalMsgs += len(f.Messages)
		totalSize += folders[i].Size

		// Check if local folder of this name exists
		lf, err := OpenLocalFolderReadOnly(localStoragePath, folderName)
		if err != nil {
			if ! (strings.HasSuffix(err.Error(), "The system cannot find the file specified.") || 
			      strings.HasSuffix(err.Error(), "The system cannot find the path specified.") )  {
				log.Fatal(err)
			}
			// fallthrough if there is no local folder
		} else {
			// Filter out messages which are already backed up locally
			defer lf.Close()
			lfm, err := lf.ReadAllIndex()
			if err != nil {
				log.Fatal(err)
			}
			f.Messages, f.Size = f.FilterOut(lfm)
		}

		filteredMsgs += len(f.Messages)
		filteredSize += f.Size
		if err := bar.Add(1); err != nil {
			log.Fatal(err)
		}
	}

	// Print overall message summary and folder details
	fmt.Println()
	fmt.Printf("%s/%s (%d/%d messages, %s/%s)\n", server, user, filteredMsgs, totalMsgs,
		humanReadableSize(filteredSize), humanReadableSize(totalSize))
	for _, f := range folders {
		fmt.Printf("|- %s (%d, %s)\n", f.Name, len(f.Messages), humanReadableSize(f.Size))
	}
	fmt.Println()

	return folders, filteredMsgs, filteredSize
}

// Backs up new messages in an IMAP account to the coresponding local storage
func cmdBackup(c *client.Client, folderNames []string) {
	folders, filteredMsgs, filteredSize := cmdQuery(c, folderNames)
	if filteredMsgs == 0 || filteredSize == 0 {
		return
	}

	// Download and append any new messages to local folder storage
	bar := pb.DefaultBytes(int64(filteredSize), "Download")
	for _, f := range folders {
		if len(f.Messages) == 0 {
			continue
		}
		bar.Describe("Download " + f.Name)

		// Open local mbox file and index file for appending
		lf, err := OpenLocalFolderAppend(localStoragePath, f.Name)
		if err != nil {
			log.Fatal(err)
		}
		defer lf.Close()

		// Download and store messages
		err = f.DownloadTo(c, lf, bar)
		if err != nil {
			log.Fatal(err)
		}
	}
}

// Deletes messages older than a given number of months from an IMAP server
func cmdDelete(c *client.Client, folderNames []string) {
	if months < 0 {
		return
	}

	now := time.Now().UTC()
	before := now.AddDate(0, -months, 0) // n months back
	ymd := "2006-01-02"
	fmt.Printf("Today is %s, deleting messages %d months or older, so before %s.\n",
		now.Format(ymd), months, before.Format(ymd))

	if !force {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Are you sure [y/n]: ")
		yn, _ := reader.ReadString('\n')
		yn = strings.TrimSpace(yn)
		if yn != "y" && yn != "Y" {
			fmt.Println("User did not confirm, aborting.")
			return
		}
	}

	bar := pb.Default(int64(len(folderNames)), "Delete")
	totalDeleted := int64(0)
	for _, folderName := range folderNames {
		bar.Describe("Delete " + folderName)
		numDeleted, err := DeleteMessagesBefore(c, folderName, before)
		if err != nil {
			log.Fatal(err)
		}
		totalDeleted += int64(numDeleted)
		if err := bar.Add(1); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Total %d message deleted\n", totalDeleted)
}

// Queries a local email storage for all folders and messages therein
func cmdLocalQuery() {
	folderNames, err := GetLocalFolderNames(localStoragePath)
	if err != nil {
		log.Fatal(err)
	}

	bar := pb.Default(int64(len(folderNames)), "Local list")
	folders := make([]*ImapFolderMeta, len(folderNames))
	totalMsgs, totalSize := uint32(0), uint64(0)

	for i, folderName := range folderNames {
		bar.Describe("Local list " + folderName)

		lf, err := OpenLocalFolderReadOnly(localStoragePath, folderName)
		if err != nil {
			log.Fatal(err)
		}
		defer lf.Close()

		folders[i], err = lf.ReadAllIndex()
		if err != nil {
			log.Fatal(err)
		}
		totalMsgs += uint32(len(folders[i].Messages))
		totalSize += folders[i].Size

		if err := bar.Add(1); err != nil {
			log.Fatal(err)
		}
	}

	// Print overall message summary and folder details
	fmt.Println()
	fmt.Printf("%s (%d messages, %s)\n", localStoragePath, totalMsgs, humanReadableSize(totalSize))
	for _, f := range folders {
		fmt.Printf("|- %s (%d, %s)\n", f.Name, len(f.Messages), humanReadableSize(f.Size))
	}
	fmt.Println()
}

// Restores folders and messages therein from local storage to an IMAP server
func cmdRestore(c *client.Client) {
	folderNames, err := GetLocalFolderNames(localStoragePath)
	if err != nil {
		log.Fatal(err)
	}

	bar := pb.Default(int64(len(folderNames)), "List")
	folders := make([]*ImapFolderMeta, len(folderNames))
	remFolders := make([]*ImapFolderMeta, len(folderNames))
	totalMsgs, totalSize := uint32(0), uint64(0)
	filteredMsgs, filteredSize := uint32(0), uint64(0)

	// Find messages in local folders which are not on the IMAP server
	for i, folderName := range folderNames {
		bar.Describe("List " + folderName)

		lf, err := OpenLocalFolderReadOnly(localStoragePath, folderName)
		if err != nil {
			log.Fatal(err)
		}
		defer lf.Close()

		folders[i], err = lf.ReadAllIndex()
		if err != nil {
			log.Fatal(err)
		}
		totalMsgs += uint32(len(folders[i].Messages))
		totalSize += folders[i].Size

		remFolders[i], err = NewImapFolderMeta(c, folderName)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "Mailbox doesn't exist") {
				log.Fatal(err)
			}
			// create folder on IMAP server if it doesn't exist
			err = c.Create(folderName)
			if err != nil {
				log.Fatal(err)
			}
			remFolders[i], err = NewImapFolderMeta(c, folderName)
			if err != nil {
				log.Fatal(err)
			}
		}
		folders[i].Messages, folders[i].Size = folders[i].FilterOut(remFolders[i])

		filteredMsgs += uint32(len(folders[i].Messages))
		filteredSize += folders[i].Size

		if err := bar.Add(1); err != nil {
			log.Fatal(err)
		}
	}

	// Print overall message summary and folder details
	fmt.Println()
	fmt.Printf("%s (%d/%d messages, %s/%s)\n", localStoragePath, filteredMsgs, totalMsgs,
		humanReadableSize(filteredSize), humanReadableSize(totalSize))
	for _, f := range folders {
		fmt.Printf("|- %s (%d, %s)\n", f.Name, len(f.Messages), humanReadableSize(f.Size))
	}
	fmt.Println()

	// Upload any new messages to IMAP server
	bar = pb.DefaultBytes(int64(filteredSize), "Upload")
	msgBuffer := &bytes.Buffer{}
	for _, f := range folders {
		bar.Describe("Upload " + f.Name)

		lf, err := OpenLocalFolderReadOnly(localStoragePath, f.Name)
		if err != nil {
			log.Fatal(err)
		}
		defer lf.Close()

		for _, mm := range f.Messages {
			if err := lf.ReadMessage(mm, msgBuffer); err != nil {
				log.Fatal(err)
			}

			l := msgBuffer.Len()
			clonedBuffer := bytes.NewBuffer(msgBuffer.Bytes())    // clone buffer so we can read it twice
			receivedTime, err := GetMessageReceived(clonedBuffer) // first read the clone here...
			if err != nil {
				log.Printf("Validity %d uid %d: Warning: Unable to parse received time, using dummy", mm.UidValidity, mm.Uid)
			}
			if err := c.Append(f.Name, nil, receivedTime, msgBuffer); err != nil { // then read the original here
				log.Fatal(err)
			}
			if err := bar.Add64(int64(l)); err != nil {
				log.Fatal(err)
			}
		}
	}

}
