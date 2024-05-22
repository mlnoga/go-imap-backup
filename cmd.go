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
	"log"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap/client"
	pb "github.com/schollz/progressbar/v3"
)

// performs the remote command given by cmd
func cmdRemote(cmd string) (err error) {
	// Connect
	bar := pb.Default(3, "Connect")
	addr := fmt.Sprintf("%s:%d", server, port)
	c, err := client.DialTLS(addr, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Logout(); err != nil {
			// cannot return a value from a deferred function when logout fails - just log it
			log.Printf("error logging out: %s", err)
		}
	}()
	if err := bar.Add(1); err != nil {
		return err
	}

	// Login
	bar.Describe("Login")
	if err := c.Login(user, pass); err != nil {
		return err
	}
	if err := bar.Add(1); err != nil {
		return err
	}

	// List folders
	bar.Describe("List folders")
	folderNames, err := ListFolders(c)
	if err != nil {
		return err
	}
	if err := bar.Add(1); err != nil {
		return err
	}

	// Restrict if necessary
	if len(restrictToFolderNames) > 0 {
		folderNames = intersect(folderNames, restrictToFolderNames)
	}

	// Execute given command
	switch cmd {
	case "query":
		_, _, _, err := cmdQuery(c, folderNames)
		return err

	case "histo":
		_, err := cmdHisto(c, folderNames, 26, 20*1024)
		return err

	case "backup":
		return cmdBackup(c, folderNames)

	case "restore":
		return cmdRestore(c)

	case "delete":
		return cmdDelete(c, folderNames)

	default:
		return fmt.Errorf("unknown command %s", cmd)
	}
}

// Queries an IMAP account for the contents of all folders with given names,
// filtering out messages already in the coresponding local storage.
// Returns a list of folders with the filtered messages therein, or err on error.
func cmdQuery(c *client.Client, folderNames []string) (folders []*ImapFolderMeta, filteredMsgs int, filteredSize uint64, err error) {
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
			return nil, 0, 0, err
		}
		f := folders[i]
		totalMsgs += len(f.Messages)
		totalSize += folders[i].Size

		// Check if local folder of this name exists
		lf, err := OpenLocalFolderReadOnly(localStoragePath, folderName)
		if err != nil {
			if !(strings.HasSuffix(err.Error(), "The system cannot find the file specified.") ||
				strings.HasSuffix(err.Error(), "The system cannot find the path specified.")) {
				return nil, 0, 0, err
			}
			// fallthrough if there is no local folder
		} else {
			// Filter out messages which are already backed up locally
			defer lf.Close()
			if lfm, err := lf.ReadAllIndex(); err != nil {
				return nil, 0, 0, err
			} else {
				f.Messages, f.Size = f.FilterOut(lfm)
			}
		}

		filteredMsgs += len(f.Messages)
		filteredSize += f.Size
		if err := bar.Add(1); err != nil {
			return nil, 0, 0, err
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

	return folders, filteredMsgs, filteredSize, nil
}

// Queries an IMAP account for the contents of all folders with given names,
// computes a histogram of message sizes. The histogram has numBins bins of
// binStrideBytes bytes each, with the last bin serving as an "or larger" bin.
// Disregards local folders. Returns histogram on success, or err on error.
func cmdHisto(c *client.Client, folderNames []string, numBins uint, binStrideBytes uint) (bins []uint, err error) {
	bins = make([]uint, numBins)
	maxMsgSize := uint(0)

	// Process all folders
	totalMsgs, totalSize := 0, uint64(0)
	bar := pb.Default(int64(len(folderNames)), "List")
	for _, folderName := range folderNames {
		bar.Describe("List " + folderName)

		// Fetch metadata for all messages in the folder
		var err error
		f, err := NewImapFolderMeta(c, folderName)
		if err != nil {
			return nil, err
		}

		totalMsgs += len(f.Messages)
		totalSize += f.Size

		// Update histogram of message sizes
		for _, m := range f.Messages {
			bin := uint(m.Size) / binStrideBytes
			if bin >= numBins {
				bin = numBins - 1
			}
			bins[bin]++
			if uint(m.Size) > maxMsgSize {
				maxMsgSize = uint(m.Size)
			}
		}

		if err := bar.Add(1); err != nil {
			return nil, err
		}
	}

	// calculate max bin value
	maxBin := uint(0)
	for _, val := range bins {
		if val > maxBin {
			maxBin = val
		}
	}

	// Print overall message summary and histogram
	fmt.Println()
	fmt.Printf("%s/%s (%d messages, %s)\n", server, user, totalMsgs, humanReadableSize(totalSize))
	fmt.Printf("Average message size is %s.\n", humanReadableSize(totalSize/uint64(totalMsgs)))
	for i, b := range bins {
		if i < len(bins)-1 {
			fmt.Printf("  <=%6s: ", humanReadableSize(uint64((i+1)*int(binStrideBytes))))
		} else {
			fmt.Printf("   >%6s: ", humanReadableSize(uint64((i)*int(binStrideBytes))))
		}

		// Print ASCII art bar chart of max width 50
		for j := uint(0); j < (b*50)/maxBin; j++ {
			fmt.Printf("█")
		}
		fmt.Printf(" %d (%.1f%%)\n", b, 100*float64(b)/float64(totalMsgs))
	}
	fmt.Printf("Maximum message size is %s.\n", humanReadableSize(uint64(maxMsgSize)))
	fmt.Println()

	return bins, nil
}

// Backs up new messages in an IMAP account to the coresponding local storage.
// Returns err on error, else nil
func cmdBackup(c *client.Client, folderNames []string) (err error) {
	folders, filteredMsgs, filteredSize, err := cmdQuery(c, folderNames)
	if err != nil {
		return err
	}
	if filteredMsgs == 0 || filteredSize == 0 {
		return nil
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
			return err
		}
		defer lf.Close()

		// Download and store messages
		err = f.DownloadTo(c, lf, bar)
		if err != nil {
			return err
		}
	}
	return nil
}

// Deletes messages older than a given number of months from an IMAP server
func cmdDelete(c *client.Client, folderNames []string) (err error) {
	if months < 0 {
		return fmt.Errorf("months must be >= 0")
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
			return fmt.Errorf("user did not confirm, aborting")
		}
	}

	bar := pb.Default(int64(len(folderNames)), "Delete")
	totalDeleted := int64(0)
	for _, folderName := range folderNames {
		bar.Describe("Delete " + folderName)
		numDeleted, err := DeleteMessagesBefore(c, folderName, before)
		if err != nil {
			return err
		}
		totalDeleted += int64(numDeleted)
		if err := bar.Add(1); err != nil {
			return err
		}
	}

	fmt.Printf("Total %d message deleted\n", totalDeleted)
	return nil
}

// Queries a local email storage for all folders and messages therein
func cmdLocalQuery() (err error) {
	folderNames, err := GetLocalFolderNames(localStoragePath)
	if err != nil {
		return err
	}

	bar := pb.Default(int64(len(folderNames)), "Local list")
	folders := make([]*ImapFolderMeta, len(folderNames))
	totalMsgs, totalSize := uint32(0), uint64(0)

	for i, folderName := range folderNames {
		bar.Describe("Local list " + folderName)

		lf, err := OpenLocalFolderReadOnly(localStoragePath, folderName)
		if err != nil {
			return err
		}
		defer lf.Close()

		folders[i], err = lf.ReadAllIndex()
		if err != nil {
			return err
		}
		totalMsgs += uint32(len(folders[i].Messages))
		totalSize += folders[i].Size

		if err := bar.Add(1); err != nil {
			return err
		}
	}

	// Print overall message summary and folder details
	fmt.Println()
	fmt.Printf("%s (%d messages, %s)\n", localStoragePath, totalMsgs, humanReadableSize(totalSize))
	for _, f := range folders {
		fmt.Printf("|- %s (%d, %s)\n", f.Name, len(f.Messages), humanReadableSize(f.Size))
	}
	fmt.Println()
	return nil
}

// Restores folders and messages therein from local storage to an IMAP server
func cmdRestore(c *client.Client) (err error) {
	folderNames, err := GetLocalFolderNames(localStoragePath)
	if err != nil {
		return err
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
			return err
		}
		defer lf.Close()

		folders[i], err = lf.ReadAllIndex()
		if err != nil {
			return err
		}
		totalMsgs += uint32(len(folders[i].Messages))
		totalSize += folders[i].Size

		remFolders[i], err = NewImapFolderMeta(c, folderName)
		if err != nil {
			if !strings.HasPrefix(err.Error(), "Mailbox doesn't exist") {
				return err
			}
			// create folder on IMAP server if it doesn't exist
			err = c.Create(folderName)
			if err != nil {
				return err
			}
			remFolders[i], err = NewImapFolderMeta(c, folderName)
			if err != nil {
				return err
			}
		}
		folders[i].Messages, folders[i].Size = folders[i].FilterOut(remFolders[i])

		filteredMsgs += uint32(len(folders[i].Messages))
		filteredSize += folders[i].Size

		if err := bar.Add(1); err != nil {
			return err
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
			return err
		}
		defer lf.Close()

		for _, mm := range f.Messages {
			if err := lf.ReadMessage(mm, msgBuffer); err != nil {
				return err
			}

			l := msgBuffer.Len()
			clonedBuffer := bytes.NewBuffer(msgBuffer.Bytes())    // clone buffer so we can read it twice
			receivedTime, err := GetMessageReceived(clonedBuffer) // first read the clone here...
			if err != nil {
				log.Printf("Validity %d uid %d: Warning: Unable to parse received time, using dummy", mm.UidValidity, mm.Uid)
			}
			if err := c.Append(f.Name, nil, receivedTime, msgBuffer); err != nil { // then read the original here
				return err
			}
			if err := bar.Add64(int64(l)); err != nil {
				return err
			}
		}
	}
	return nil
}
