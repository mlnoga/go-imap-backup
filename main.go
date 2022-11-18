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
var localStoragePath string
var restrictToFoldersSeparated string
var restrictToFolderNames []string
var months int
var force bool

func init() {
	flag.Usage = func() {
		o := flag.CommandLine.Output()
		fmt.Fprintln(o, "Usage: go-imap-backup [-flags] command, where command is one of:")
		fmt.Fprintln(o, "  query:   fetch folder and message overview from IMAP server")
		fmt.Fprintln(o, "  lquery:  fetch folder and message metadata from local storage")
		fmt.Fprintln(o, "  backup:  save new messages on IMAP server to local storage")
		fmt.Fprintln(o, "  restore: restore messages from local storage to IMAP server")
		fmt.Fprintln(o, "  delete:  delete older messages from IMAP server")
		fmt.Fprintln(o, "")
		fmt.Fprintln(o, "The available flags are:")
		flag.PrintDefaults()
	}

	flag.StringVar(&server, "s", "", "IMAP server name")
	flag.IntVar(&port, "p", 993, "IMAP port number")
	flag.StringVar(&user, "u", "", "IMAP user name")
	flag.StringVar(&pass, "P", "", "IMAP password. Really, consider entering this into stdin")
	flag.StringVar(&localStoragePath, "l", "", "Local storage path, defaults to (server)/(user)")
	flag.IntVar(&months, "m", 24, "Age limit for deletion in months, must be non-negative")
	flag.BoolVar(&force, "f", false, "Force deletion of older messages without confirmation prompt")
	flag.StringVar(&restrictToFoldersSeparated, "r", "", "Restrict command to a comma-separated list of folders")
}

func main() {
	// parse command-line arguments, and complete for local commands
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || (args[0] != "query" && args[0] != "lquery" && args[0] != "backup" &&
		args[0] != "restore" && args[0] != "delete") {
		flag.Usage()
		return
	}

	// perform local command, if given
	switch args[0] {
	case "lquery":
		if err := completeFlagsLocal(); err != nil {
			log.Fatal(err)
		}
		cmdLocalQuery()
		return
	}

	// complete flags for remote operations
	if err := completeFlagsRemote(); err != nil {
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

	case "restore":
		cmdRestore(c)

	case "delete":
		cmdDelete(c, folderNames)
	}

	fmt.Println("Done, exiting.")
}

// Validate command line flags for local commands, and prompt for missing parameters
func completeFlagsLocal() (err error) {
	if localStoragePath == "" {
		if server != "" && user != "" {
			localStoragePath = server + "/" + user
		} else {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Local storage path: ")
			localStoragePath, _ = reader.ReadString('\n')
			localStoragePath = strings.TrimSpace(localStoragePath)
		}
	}

	return nil
}

// Validate command line flags for remote commands, and prompt for missing parameters
func completeFlagsRemote() (err error) {
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

	if localStoragePath == "" {
		localStoragePath = server + "/" + user
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

	if months < 0 {
		return fmt.Errorf("Months must be non-negative, is %d", months)
	}

	restrictToFolderNames = strings.Split(restrictToFoldersSeparated, ",")
	if len(restrictToFolderNames) == 1 && restrictToFolderNames[0] == "" {
		restrictToFolderNames = nil
	}

	return nil
}
