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
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"golang.org/x/term"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	progressbar "github.com/schollz/progressbar/v3"
)

// Metadata for an email message on IMAP server or in a local .mbox file
type MsgMetaData struct {
	SeqNum      uint32   // sequence number >=1 on IMAP server, or 0 if unknown
    UidValidity uint32
    Uid         uint32
	Size        uint32
	Offset      uint64   // offset in bytes in local .mbox file, or math.MaxUint64 if unknown
}

func (md *MsgMetaData) GetUuid() uint64 {
	return (uint64(md.UidValidity)<<32) | uint64(md.Uid)
}

// command line flag values
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

	if folderNames==nil || (len(folderNames)==1 && folderNames[0]=="") {
		folderNames=listFolders(c)
	}

	// Traverse folders 
	log.Printf("Processing %d folders ...", len(folderNames))
	for _,folderName := range folderNames {
		mds, _:=fetchMsgMetaData(c, folderName)

		// Retrieve and locally back up messages
		mdsMap, uidValidity:=readMailboxIndex(folderName)
		log.Printf("  - %d messages in local backup, last UidValidity %d", len(mdsMap), uidValidity)
		mdsNew, size:=filterNewMsgMetaData(mds, mdsMap)
		log.Printf("  - %d new messages with %s to be added", 
			       len(mdsNew), humanReadableSize(size))
		downloadAndSaveMessages(c, mdsNew, folderName)

		// Optionally delete older messages
		if months>0 {
			now:=time.Now()
			before:=now.AddDate(0, -months, 0) // n months back
			ymd:="2006-01-02"
			log.Printf("Today is %s, before %d months was %s", 
				        now.Format(ymd), months, before.Format(ymd)) 
			//deleteMessagesBefore(c, f, before)
		}
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
	flag.IntVar   (&months, "m", -1,  "Delete messages older than this amount of months, if >=0")	
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


func listFolders(c *client.Client) []string {
	log.Printf("Fetching folder list...")
	// Query list of folders
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

	log.Printf("Found %d folders:\n", len(mailboxes))
	for _,m :=range mailboxes {
		log.Printf("- %s\n", m)
	}
	if err := <-done; err != nil {
		log.Fatal(err)
	}
	return mailboxes
}


func fetchMsgMetaData(c *client.Client, folderName string) (mds []MsgMetaData, uidValidity uint32) {
	log.Println("- Opening folder " + folderName)
	mbox, err := c.Select(folderName, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("  - Folder has %d messages, UID validity %d.\n", mbox.Messages, mbox.UidValidity)
	if mbox.Messages==0 {
		return nil, mbox.UidValidity
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(1, mbox.Messages)
	items  := []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size}

	messages := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() {
	    done <- c.Fetch(seqset, items, messages)
	}()

	messageDigests:=[]MsgMetaData{}
	totalSize:=uint64(0)
	for msg := range messages {
		d:=MsgMetaData{SeqNum: msg.SeqNum, UidValidity: mbox.UidValidity, Uid: msg.Uid, Size: msg.Size, Offset: math.MaxUint64}
		messageDigests=append(messageDigests, d)
		totalSize+=uint64(msg.Size)
	}
	log.Printf("  - Found %d id/uid pairs, size on server %s", len(messageDigests), humanReadableSize(totalSize))
	if err := <-done; err != nil {
		log.Fatal(err)
	}

	return messageDigests, mbox.UidValidity
}


// Read mailbox index file and return a map of 64-bit message IDs to message metadata
func readMailboxIndex(folderName string) (res map[uint64]MsgMetaData, uidValidity uint32) {
	res=make(map[uint64]MsgMetaData)

	// open input file readonly
	fileName:=folderName+".idx"
	f, err:=os.Open(fileName)
	if err!=nil {
		// silently return blank list if file does not exist
		return res, 0
	}
	defer f.Close()

	// read line by line
	lineNo:=1
	s:=bufio.NewScanner(f)
	for s.Scan() {
		line:=s.Text() // without terminating newline

		// split line into fields
		md:=MsgMetaData{SeqNum:0, Offset: math.MaxUint64}
		_,err:=fmt.Sscanf(line, "%d\t%d\t%d\t%d", 
		                  &md.UidValidity, &md.Uid, &md.Size, &md.Offset)
		if err!=nil {
			log.Fatalf("%s:%d: %s", fileName, lineNo, err.Error())
		}
		uidValidity=md.UidValidity

		// insert into results map
		res[md.GetUuid()]=md
		lineNo++
	}
	if err:=s.Err(); err!=nil {
		log.Fatalf("%s:%d: %s", fileName, lineNo, err.Error())
	}

	return res, uidValidity
}


func filterNewMsgMetaData(mds []MsgMetaData, lookup map[uint64]MsgMetaData) (res []MsgMetaData, size uint64) {
	res=[]MsgMetaData{}
	size=0
	for _, md := range(mds) {
		if _,ok :=lookup[md.GetUuid()]; !ok {
			res=append(res, md)
			size+=uint64(md.Size)
		}
	}
	return res, size
}

func downloadAndSaveMessages(c *client.Client, mds []MsgMetaData, folderName string) {
	if len(mds)==0 {
		return
	}

	// open mailbox file for appending
	mboxName:=folderName+".mbox"
	mbox, err:=os.OpenFile(mboxName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err!=nil {
		log.Fatal(err)
	}
	defer mbox.Close()

	// open mailbox index file for appending
	idxName:=folderName+".idx"
	idx, err:=os.OpenFile(idxName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err!=nil {
		log.Fatal(err)
	}
	defer idx.Close()
	idxWriter:=bufio.NewWriter(idx)
	defer idxWriter.Flush()

	// prepare sequence set and trigger download of messages
	totalSize:=uint64(0)
	seqset := new(imap.SeqSet)
	for _,md:=range(mds) {
		seqset.AddNum(md.SeqNum)
		totalSize+=uint64(md.Size)
	}
	uidValidity:=mds[0].UidValidity

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size, imap.FetchEnvelope, section.FetchItem()}	

	bar := progressbar.DefaultBytes(int64(totalSize), folderName)
	messages := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() {
	    done <- c.Fetch(seqset, items, messages)
	}()

	// process messages received
	for msg := range messages {
		// print progress
		bar.Add64(int64(msg.Size))
		
		// read message into memory
		r := msg.GetBody(section)
		if r == nil {
		    log.Fatal("Server didn't return message body")
		}
		bs, err:=io.ReadAll(r)
		if err!=nil {
		    log.Fatal(err)
		}

		// write header into mbox file
		header:=fmt.Sprintf("From %s %s\n", msg.Envelope.From[0].Address(), msg.Envelope.Date.UTC().Format(time.ANSIC))
		_,err=fmt.Fprintf(mbox,"%s", header)
		if err!=nil {
		    log.Fatal(err)
		}

		// retrieve current mbox file size in bytes, for storing in index file
		pos, err:=mbox.Seek(0, os.SEEK_CUR)
		if err!=nil {
		    log.Fatal(err)
		}

		// write message body into mbox file
		_, err=mbox.Write(bs)
		if err!=nil {
		    log.Fatal(err)
		}

		// write separating blank line into mbox file
		_,err=fmt.Fprintf(mbox,"\n")
		if err!=nil {
		    log.Fatal(err)
		}

		// write corresponding index record to idx file
		fmt.Fprintf(idxWriter, "%d\t%d\t%d\t%d\n", uidValidity, msg.Uid, len(bs), pos)
	}
	if err := <-done; err != nil {
		log.Fatal(err)
	}
}

func deleteMessagesBefore(c *client.Client, folderName string, before time.Time) {
	log.Println("- Opening folder " + folderName)
	mbox, err := c.Select(folderName, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("  - Folder has %d messages, UID validity %d.\n", mbox.Messages, mbox.UidValidity)
	if mbox.Messages==0 {
		return
	}

	log.Printf("  - Finding messages before %v ...", before)
	ids := findMessagesBefore(c, before)
	log.Printf("  - Found %d messages", len(ids))
	if len(ids)==0 {
		return
	}

	log.Printf("  - Deleting %d messsages ...", len(ids))
	deleteMessages(c, ids)

	mbox, err = c.Select(folderName, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("  - Folder has %d messages, UID validity %d.\n", mbox.Messages, mbox.UidValidity)
}


func findMessagesBefore(c *client.Client, before time.Time) []uint32 {
	criteria:=imap.NewSearchCriteria()
	criteria.Before=before
	ids, err := c.Search(criteria)
	if err!=nil {
		log.Fatal(err)
	}
	return ids
}


func deleteMessages(c *client.Client, ids []uint32) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)

	item:=imap.FormatFlagsOp(imap.AddFlags, true)			
	flags:=[]interface{}{imap.DeletedFlag}
	if err:=c.Store(seqset, item, flags, nil); err!=nil {
		log.Fatal(err)
	}

	if err:=c.Expunge(nil); err!= nil {
		log.Fatal(err)
	}
}


func humanReadableSize(n uint64) string {
	if n<1024 { 
		return fmt.Sprintf("%d B", n) 
	} else if n<10*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024) 
	} else if n<1024*1024 {
		return fmt.Sprintf("%d KB", n/1024) 
	} else if n<10*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024) 
	} else if n<1024*1024*1024 {
		return fmt.Sprintf("%d MB", n/1024/1024) 
	} else if n<10*1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(n)/1024/1024/1024) 
	} else if n<1024*1024*1024*1024 {
		return fmt.Sprintf("%d GB", n/1024/1024/1024) 
	} else if n<10*1024*1024*1024*1024 {
		return fmt.Sprintf("%.1f TB", float64(n)/1024/1024/1024/1024) 
	} else {
		return fmt.Sprintf("%d TB", n/1024/1024/1024/1024) 
	}
}