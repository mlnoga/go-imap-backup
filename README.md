# go-imap-backup

Backup emails from an IMAP server to local storage, optionally deleting older messages.


## Usage

`go-imap-backup [-flags] command`, where `command` is one of:

* `query` fetch folder and message overview from IMAP server
* `backup` save new messages on IMAP server to local storage
* `delete` delete older messages from IMAP server
* `lquery` etch folder and message metadata from local storage

The available flags are:

| Flag  | Description         | Default             |
|-------|---------------------|---------------------|
| -s    | IMAP server name    | (read from console) |
| -p    | IMAP port number    | 993                 |
| -u    | IMAP user name      | (read from console) |
| -P    | IMAP password       | (read from console) |
| -m    | Age limit for deletion in months, must be positive | 24 | 
| -f    | Force deletion of older messages without confirmation prompt | false |
| -r    | Restrict command to a comma-separated list of folders | (blank) | 


## Local storage

Backups are stored locally in a directory tree `server/user/`, which is created by the backup command if necessary. For each folder on the IMAP server, the local directory contains both a mailbox file named `folder.mbox`, and an index of the messages therein called `folder.idx`. 

The `.mbox` files follow `mboxo` format as defined [here](https://en.wikipedia.org/wiki/Mbox). That is, they do not quote lines starting with `From `. This preserves message sizes, checksums and signature validities. The backup tool avoids ambiguities arising from this by always addressing the `.mbox` file according to the indices and offsets in the corresponding `.idx` file.

The `.idx` file is a text file with one newline-separated line per message. Each line consists of the following tab-separated columns:

| Column | Description |
|--------|-------------|
| UidValidity | A unique 32-bit integer identifier for an Imap folder |
| Uid         | A unique 32-bit integer identifier for a message inside an Imap folder |
| Size        | The size of the email message in bytes |
| Offset      | The starting offset of the email message in the `.mbox` file |

Note that the offset points directly at the start of the message itself, not at the separator line `From abc@def.com timestamp` preceding it in the `.mbox` file. The size is the exact size of the message as well, excluding the blank separator line following the message in the `.mbox` file.


## License

[GPL v3](https://www.gnu.org/licenses/gpl-3.0.en.html)


## Libraries used

This project uses a number of open source libraries. Please refer to their respective
repositories for licensing terms.

* [golang.org/x/sys](https://golang.org/x/sys) (indirect)
* [golang.org/x/term](https://golang.org/x/term)
* [golang.org/x/text](https://golang.org/x/text) (indirect)
* [github.com/emersion/go-imap](https://github.com/emersion/go-imap)
* [github.com/emersion/go-sasl](https://github.com/emersion/go-sasl) (indirect)
* [github.com/schollz/progressbar](https://github.com/schollz/progressbar)