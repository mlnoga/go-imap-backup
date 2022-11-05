// go-imap-deleter (C) 2022 by Markus L. Noga
// Connect to an IMAP server and delete older messages
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
	"log"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap"
	"fmt"
	"os"
	"bufio"
	"strings"
	"time"
	"sort"
	"flag"
	"golang.org/x/term"
)

var server        string
var port          int
var user          string
var pass          string
var folderNames []string
var months        int

func main() {
	processFlags()

	// Connect to server
	addr := fmt.Sprintf("%s:%d", server, port)
	log.Printf("Connecting to server %s ...\n", addr)
	c, err := client.DialTLS(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Logout()
	log.Println("Connected")

	// Login
	log.Printf("Logging in as user %s ...\n", user)
	if err := c.Login(user, pass); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")

	// Query list of mailboxes
	mailboxesCh := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func () {
		done <- c.List("", "*", mailboxesCh)
	}()

	// Collect results and print 
	mailboxes:=[]string{}
	for m := range mailboxesCh {
		mailboxes=append(mailboxes, m.Name)
	}
	sort.Strings(mailboxes)

	log.Printf("Found %d mailboxes:\n", len(mailboxes))
	for _,m :=range mailboxes {
		log.Printf("- %s\n", m)
	}
	if err := <-done; err != nil {
		log.Fatal(err)
	}

	// Determine cutoff time based on given number of months
	now:=time.Now()
	cutoff:=now.AddDate(0, -months, 0) // n months back
	log.Printf("Current date is %v, archiving messages older than %d months i.e. before %v",
	           now, months, cutoff)

	// Traverse mailboxes 
	log.Printf("Processing %d mailboxes ...", len(folderNames))
	for _,f := range folderNames {
		processFolder(c, f, cutoff)
	}

	log.Println("Done, exiting.")
}

func processFlags() {
	flag.StringVar(&server, "s", "",  "IMAP server name")
	flag.IntVar   (&port,   "p", 993, "IMAP port number")
	flag.StringVar(&user,   "u", "",  "IMAP user name")
	flag.StringVar(&pass,   "P", "",  "IMAP password. Really, consider entering this into stdin")
	var folderNamesSeparated string
	flag.StringVar(&folderNamesSeparated, "f", "INBOX,INBOX.Drafts,INBOX.Sent,INBOX.Spam,INBOX.Trash", "Comma-separated list of folders to work on")
	flag.IntVar   (&months, "m", 24,  "Delete messages older than this amount of months")	
	flag.Parse()
	folderNames=strings.Split(folderNamesSeparated,",")

	reader:=bufio.NewReader(os.Stdin)
	if server=="" {
 		fmt.Printf("IMAP server: ")
		server,_:=reader.ReadString('\n')
		server=strings.TrimSpace(server)
	}
	if user=="" {
 		fmt.Printf("Username: ")
		user,_:=reader.ReadString('\n')
		user=strings.TrimSpace(user)
	}
	if pass=="" {
 		fmt.Printf("Password: ")
		// Read password from terminal without echoing it
		oldState, err :=term.MakeRaw(int(os.Stdin.Fd()))
		if err!=nil {
			log.Fatal(err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)

		t:=term.NewTerminal(os.Stdin, "")
		p, err := t.ReadPassword("")
		if err!=nil {
			log.Fatal(err)
		}
		pass=string(p)
		fmt.Println()
	}
}


func processFolder(c *client.Client, f string, cutoff time.Time) {
	log.Println("- Opening folder " + f)
	mbox, err := c.Select(f, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("  - Folder has %d messages, %d new and %d unread.\n", 
		       mbox.Messages, mbox.Recent, mbox.Unseen)

	if mbox.Messages>0 {
		log.Printf("  - Searching messages older than %v: ", cutoff)
		id, ok:=findYoungestMessageOlderOrEqualThan(c, mbox.Messages, cutoff)
		if !ok {
			log.Printf("  - No such messages found\n")
			return
		}
		log.Printf("  - Deleting messsages [1,%d] ...", id)
		deleteMessageRange(c, 1, id)
	}
}


func findYoungestMessageOlderOrEqualThan(c *client.Client, numMsgs uint32, date time.Time) (id uint32, ok bool) {
	left  := uint32(1)
	right := numMsgs

	for left < right {
		middle := (left + right + 1) / 2
		midMsg := fetchMessageByIndex(c, middle)
		midDate:= midMsg.Envelope.Date

		// fmt.Printf("[%d,%d] mid %d date %v subject %s\n", left, right, middle, midDate, midMsg.Envelope.Subject)

		if midDate.After(date) {
			right = middle - 1
		} else {
			left = middle
		}
	}

	msg:=fetchMessageByIndex(c, left)	
	if msg.Envelope.Date.After(date) {
		return 0, false
	}
	return left, true
}


func fetchMessageByIndex(c *client.Client, id uint32) *imap.Message {
	seqset := new(imap.SeqSet)
	seqset.AddRange(id, id)

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
	}()

	if err := <-done; err != nil {
		log.Fatal(err)
	}

	return <- messages
}


func deleteMessageRange (c *client.Client, from, to uint32) {
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	item:=imap.FormatFlagsOp(imap.AddFlags, true)			
	flags:=[]interface{}{imap.DeletedFlag}
	if err:=c.Store(seqset, item, flags, nil); err!=nil {
		log.Fatal(err)
	}

	if err:=c.Expunge(nil); err!= nil {
		log.Fatal(err)
	}
}