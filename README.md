# go-imap-backup

Backs up messages from an IMAP server to local files, optionally deleting older messages.

Backups are stored locally in a directory `server/user/`, which is created if necessary. For each folder on the server, the directory contains a mailbox file named `folder.mbox`, and an index of the messages therein in a file called `folder.idx`. 

Newly identified messages on the server are appended to the mbox file. Messages deleted from the server are not removed from the mbox file, because it is intended to function as a permanent archive.


## Usage

`go-imap-backup [-flags] command` where commands are:

* `query` retrieves a summary of the messages on the server, and takes no further action
* `backup` retrieves new messages from the server, and appends them to the local backup folders
* `delete` removes messages older than the given amount of months from the server

The corresponding flags are:

| Flag  | Description         | Default             |
|-------|---------------------|---------------------|
| -s    | IMAP server name    | (read from console) |
| -p    | IMAP port address   | 993                 |
| -u    | IMAP user name      | (read from console) |
| -P    | IMAP password       | (read from console) |
| -f    | Force deletion without confirmation prompt | false |
| -m    | Age limit for deletion in months, must be positive | 24 | 
| -r    | Restrict to comma-separated list of folders | (blank) | 

## License

GPL v3


## Libraries used

This project uses a number of open source libraries. Please refer to their respective
repositories for licensing terms.

* [golang.org/x/sys](https://golang.org/x/sys) (indirect)
* [golang.org/x/term](https://golang.org/x/term)
* [golang.org/x/text](https://golang.org/x/text) (indirect)
* [github.com/emersion/go-imap](https://github.com/emersion/go-imap)
* [github.com/emersion/go-sasl](https://github.com/emersion/go-sasl) (indirect)
* [github.com/schollz/progressbar](https://github.com/schollz/progressbar)