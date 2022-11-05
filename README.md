# go-imap-deleter

Delete older messages from an IMAP server.

## Command-line flags

| Flag  | Description         | Default             |
|-------|---------------------|---------------------|
| -s    | IMAP server name    | (read from console) |
| -p    | IMAP port address   | 993                 |
| -u    | IMAP user name      | (read from console) |
| -P    | IMAP password       | (read from console) |
| -f    | Comma-separated list of folders | INBOX,INBOX.Drafts,INBOX.Sent,INBOX.Spam,INBOX.Trash | 
| -m    | Age limit in months | 24 | 


## Libraries used

This project uses a number of open source libraries. Please refer to their respective
repositories for licensing terms.

* github.com/emersion/go-imap
* github.com/emersion/go-sasl (indirect)
* golang.org/x/sys (indirect)
* golang.org/x/term
* golang.org/x/text(indirect)