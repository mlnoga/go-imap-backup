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
	"flag"
	"fmt"
	"github.com/emersion/go-imap/client"
	pb "github.com/schollz/progressbar/v3"
	"golang.org/x/term"
	"log"
	"os"
	"strings"
	"time"
)

// command line flag values
var server string
var port int
var user string
var pass string
var restrictToFoldersSeparated string
var restrictToFolderNames []string
var months int
var force bool

func init() {
	flag.Usage = func() {
		o := flag.CommandLine.Output()
		fmt.Fprintln(o, "Usage: go-imap-backup [-flags] command, where command is one of:")
		fmt.Fprintln(o, "  query:  fetch metadata from Imap server")
		fmt.Fprintln(o, "  backup: backup Imap server to local folders")
		fmt.Fprintln(o, "  delete: delete older messages from Imap server")
		fmt.Fprintln(o, "  lquery: fetch metadata from local folders")
		fmt.Fprintln(o, "")
		fmt.Fprintln(o, "The available flags are:")
		flag.PrintDefaults()
	}

	flag.StringVar(&server, "s", "", "IMAP server name")
	flag.IntVar(&port, "p", 993, "IMAP port number")
	flag.StringVar(&user, "u", "", "IMAP user name")
	flag.StringVar(&pass, "P", "", "IMAP password. Really, consider entering this into stdin")
	flag.StringVar(&restrictToFoldersSeparated, "r", "", "Restrict command to a comma-separated list of folders")
	flag.IntVar(&months, "m", 24, "Delete messages older than this amount of months")
	flag.BoolVar(&force, "f", false, "Force deletion of older messages without confirmation prompt")
}

func main() {
	// parse command-line arguments
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || (args[0] != "query" && args[0] != "lquery" && args[0] != "backup" && args[0] != "delete") {
		flag.Usage()
		return
	}

	switch args[0] {
	case "lquery":
		cmdLocalQuery()
		return
	}

	// complete flags for remote operations
	if err := completeFlags(); err != nil {
		log.Fatal(err)
	}

	// Connect
	bar := pb.Default(3, "Connect")
	addr := fmt.Sprintf("%s:%d", server, port)
	c, err := client.DialTLS(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := c.Logout(); err != nil {
			log.Fatal(err)
		}
	}()
	if err := bar.Add(1); err != nil {
		log.Fatal(err)
	}

	// Login
	bar.Describe("Login")
	if err := c.Login(user, pass); err != nil {
		log.Fatal(err)
	}
	if err := bar.Add(1); err != nil {
		log.Fatal(err)
	}

	// List folders
	bar.Describe("List folders")
	folderNames, err := ListFolders(c)
	if err != nil {
		log.Fatal(err)
	}
	if err := bar.Add(1); err != nil {
		log.Fatal(err)
	}

	// Restrict if necessary
	if len(restrictToFolderNames) > 0 {
		folderNames = intersect(folderNames, restrictToFolderNames)
	}

	// Execute given command
	switch args[0] {
	case "query":
		cmdQuery(c, folderNames)

	case "backup":
		cmdBackup(c, folderNames)

	case "delete":
		cmdDelete(c, folderNames)
	}

	fmt.Println("Done, exiting.")
}

// Validate command line flags and prompt for missing parameters (e.g. password)
func completeFlags() (err error) {
	if months <= 0 {
		return fmt.Errorf("Months must be positive, is %d", months)
	}

	restrictToFolderNames = strings.Split(restrictToFoldersSeparated, ",")
	if len(restrictToFolderNames) == 1 && restrictToFolderNames[0] == "" {
		restrictToFolderNames = nil
	}

	reader := bufio.NewReader(os.Stdin)
	if server == "" {
		fmt.Printf("IMAP server: ")
		server, _ = reader.ReadString('\n')
		server = strings.TrimSpace(server)
	}
	if user == "" {
		fmt.Printf("Username: ")
		user, _ = reader.ReadString('\n')
		user = strings.TrimSpace(user)
	}
	if pass == "" {
		fmt.Printf("Password: ")
		// Read password from terminal without echoing it
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return err
		}
		defer func() {
			if dErr := term.Restore(int(os.Stdin.Fd()), oldState); dErr != nil {
				if err == nil {
					err = dErr
				}
			}
		}()

		t := term.NewTerminal(os.Stdin, "")
		p, err := t.ReadPassword("")
		if err != nil {
			return err
		}
		pass = string(p)
		fmt.Println()
	}
	return nil
}

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

		// Find out which messages are stored locally
		lf, err := OpenLocalFolderReadOnly(server, user, folderName)
		if err != nil {
			log.Fatal(err)
		}
		defer lf.Close()
		lfm, err := lf.ReadAllIndex()
		if err != nil {
			log.Fatal(err)
		}

		// Filter out messages which are already backed up locally
		f.Messages, f.Size = f.FilterOut(lfm)
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
		lf, err := OpenLocalFolderAppend(server, user, f.Name)
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

func cmdDelete(c *client.Client, folderNames []string) {
	if months <= 0 {
		return
	}

	now := time.Now()
	before := now.AddDate(0, -months, 0) // n months back
	ymd := "2006-01-02"
	fmt.Printf("Today is %s, deleting messages %d months or older, so on or before %s.\n",
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

func cmdLocalQuery() {
	folderNames, err := GetLocalFolderNames(server, user)
	if err != nil {
		log.Fatal(err)
	}

	bar := pb.Default(int64(len(folderNames)), "Local query")
	folders := make([]*ImapFolderMeta, len(folderNames))
	totalMsgs, totalSize := uint32(0), uint64(0)

	for i, folderName := range folderNames {
		bar.Describe("Local query " + folderName)

		lf, err := OpenLocalFolderReadOnly(server, user, folderName)
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
	fmt.Printf("%s/%s (%d messages, %s)\n", server, user, totalMsgs, humanReadableSize(totalSize))
	for _, f := range folders {
		fmt.Printf("|- %s (%d, %s)\n", f.Name, len(f.Messages), humanReadableSize(f.Size))
	}
	fmt.Println()
}

// For future implementation of the "restore" command:
//
// msgCount:=0
// for lf.MboxScan() {
// 	msg:=lf.MboxText()
// 	msgCount++
// 	size:=len(msg)
// 	printSize:=size
// 	if printSize>400 {
// 		printSize=400
// 	}
// 	fmt.Printf("Message %d len %d first bytes: %s\n\n", msgCount, size, string(msg[:printSize]))
// }
// if err:=lf.MboxErr(); err!=nil {
// 	log.Fatal(err)
// }
