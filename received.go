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
	"fmt"
	message "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"io"
	"strings"
	"time"
)

// Parses given bytes as an email message, and returns the timestamp
// at the end of the first "Received" header as a go time.Time value.
// Returns empty time value time.Time{} if err is non-nil.
func GetMessageReceived(r io.Reader) (t time.Time, err error) {
	m, err := message.Read(r)
	if err != nil {
		return time.Time{}, err
	}
	fields := m.Header.FieldsByKey("Received")
	if !fields.Next() {
		return time.Time{}, fmt.Errorf("Missing Received field in message")
	}
	receivedValue, err := fields.Text()
	if err != nil {
		return time.Time{}, err
	}
	splits := strings.Split(receivedValue, ";")
	if len(splits) < 2 {
		return time.Time{}, fmt.Errorf("Received field lacks semicolon: %s", receivedValue)
	}
	timeString := strings.TrimSpace(splits[len(splits)-1])
	t, err = time.Parse(time.RFC1123Z, timeString)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
