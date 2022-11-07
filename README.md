# go-imap-backup

Backs up messages from an IMAP server, optionally deleting older messages.

Backups are stored locally in one mbox file for each folder on the server, named `folder.mbox`. The contents of the mbox file are indexed in the corresponding file `folder.idx`.

Newly identified messages on the server are appended to the mbox file. Messages deleted from the server are not removed from the mbox file, because it is intended to function as a permanent archive.


## Command-line flags

| Flag  | Description         | Default             |
|-------|---------------------|---------------------|
| -s    | IMAP server name    | (read from console) |
| -p    | IMAP port address   | 993                 |
| -u    | IMAP user name      | (read from console) |
| -P    | IMAP password       | (read from console) |
| -f    | Comma-separated list of folders | INBOX,INBOX.Drafts,INBOX.Sent,INBOX.Spam,INBOX.Trash | 
| -m    | Age limit in months | -1 (do not delete messages) | 


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