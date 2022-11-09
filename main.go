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
)

// command line flag values
var server string
var port int
var user string
var pass string
var folderNames []string
var months int

func main() {
	if err := processFlags(); err != nil {
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
	folderNames, err = ListFolders(c)
	if err != nil {
		log.Fatal(err)
	}
	if err := bar.Add(1); err != nil {
		log.Fatal(err)
	}

	// Process all folders
	bar = pb.Default(int64(len(folderNames)), "List")
	folders := make([]*ImapFolderMeta, len(folderNames))
	totalMsgs, totalSize := 0, uint64(0)
	filteredMsgs, filteredSize := 0, uint64(0)
	for i, folderName := range folderNames {
		bar.Describe("List " + folderName)

		// Fetch metadata for all messages in the folder
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
		mdsMap, _, err := lf.ReadAllIndex()
		if err != nil {
			log.Fatal(err)
		}

		// Filter out messages which are already backed up locally
		f.Messages, f.Size = filterNewMsgMetaData(f.Messages, mdsMap)
		filteredMsgs += len(f.Messages)
		filteredSize += f.Size
		if err := bar.Add(1); err != nil {
			log.Fatal(err)
		}
	}

	// Print overall message summary and folder details
	fmt.Println()
	fmt.Printf("%s/%s (%d/%d msg, %s/%s)\n", server, user, filteredMsgs, totalMsgs,
		humanReadableSize(filteredSize), humanReadableSize(totalSize))
	for _, f := range folders {
		fmt.Printf("|- %s (%d, %s)\n", f.Name, len(f.Messages), humanReadableSize(f.Size))
	}
	fmt.Println()

	// Download and append any new messages to local folder storage
	if filteredMsgs > 0 {
		bar = pb.DefaultBytes(int64(filteredSize), "Download")
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

	//	if folderNames==nil || (len(folderNames)==1 && folderNames[0]=="") {
	//		folderNames=listFolders(c)
	//	}

	// Optionally delete older messages
	// if months>0 {
	// 	now:=time.Now()
	// 	before:=now.AddDate(0, -months, 0) // n months back
	// 	ymd:="2006-01-02"
	// 	fmt.Printf("Today is %s, deleting messages older than %d months, i.e. %s or earlier",
	// 		        now.Format(ymd), months, before.Format(ymd))

	// 	bar=pb.DefaultBytes(len(folders), "Delete")
	// 	totalDeleted:=int64(0)
	// 	for f := range folders {
	// 		bar.Describe("Delete "+f.Name)
	// 		numDeleted, err:=DeleteMessagesBefore(c, f.Name, before)
	//      if err!=nil { log.Fatal(err) }
	//      totalDeleted+=int64(numDeleted)
	// 		bar.Add(1)
	// 	}
	//  fmt.Printf("Total %d message deleted\n", totalDeleted)
	// }

	fmt.Println("Done, exiting.")
}

// Process command line flags with sensible defaults,
// prompting for missing required values (e.g. password)
func processFlags() (err error) {
	flag.StringVar(&server, "s", "", "IMAP server name")
	flag.IntVar(&port, "p", 993, "IMAP port number")
	flag.StringVar(&user, "u", "", "IMAP user name")
	flag.StringVar(&pass, "P", "", "IMAP password. Really, consider entering this into stdin")
	var folderNamesSeparated string
	flag.StringVar(&folderNamesSeparated, "f", "INBOX,INBOX.Drafts,INBOX.Sent,INBOX.Spam,INBOX.Trash", "Comma-separated list of folders to work on")
	flag.IntVar(&months, "m", -1, "Delete messages older than this amount of months, if >=0")
	flag.Parse()
	folderNames = strings.Split(folderNamesSeparated, ",")

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
