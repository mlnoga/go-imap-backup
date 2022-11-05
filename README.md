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


## License

GPL v3


## Libraries used

This project uses a number of open source libraries. Please refer to their respective
repositories for licensing terms.

* [github.com/emersion/go-imap](https://github.com/emersion/go-imap)
* [github.com/emersion/go-sasl](https://github.com/emersion/go-sasl) (indirect)
* [golang.org/x/sys](https://golang.org/x/sys) (indirect)
* [golang.org/x/term](https://golang.org/x/term)
* [golang.org/x/text](https://golang.org/x/text) (indirect)