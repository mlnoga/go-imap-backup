# go-imap-deleter

Delete older messages from an IMAP server.

Command-line flags:

| Flag  | Default  | Description |
|-------|----------|-------------|
| -s | <read from standard input> | IMAP server name |
| -p | 993                        | IMAP port address |
| -u | <read from standard input> | IMAP user name |
| -P | <read from standard input> | IMAP password |
| -f | INBOX,INBOX.Drafts,INBOX.Sent,INBOX.Spam,INBOX.Trash | Comma-separated list of folders to process |
| -m | 24 | Delete messages older than this amount of months |
