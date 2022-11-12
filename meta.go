// go-imap-backup (C) 2022 by Markus L. Noga
// Backup messages from an IMAP server, optionally deleting older messages
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

// Metadata for a folder and its messages on an IMAP server or in a local file
type ImapFolderMeta struct {
	Name        string
	UidValidity uint32
	Messages    []MessageMeta
	Size        uint64 // total size of all messages in bytes
}

// Metadata for an email message on an IMAP server or in a local file
type MessageMeta struct {
	SeqNum      uint32 // sequence number >=1 on IMAP server, or 0 if unknown
	UidValidity uint32
	Uid         uint32
	Size        uint32
	Offset      uint64 // offset in bytes in local .mbox file, or math.MaxUint64 if unknown
}

// Create an 64-bit unique identifier from the folder Uid validity and the message Uid
func (md *MessageMeta) GetUuid() uint64 {
	return (uint64(md.UidValidity) << 32) | uint64(md.Uid)
}

// From a list of messages, filter out all messages in the given map,
// returning a new list of messages and total size of the messages in bytes.
func (f *ImapFolderMeta) FilterOut(out *ImapFolderMeta) (res []MessageMeta, size uint64) {
	outMap := out.GetMap()

	res = []MessageMeta{}
	size = 0
	for _, md := range f.Messages {
		if _, ok := outMap[md.GetUuid()]; !ok {
			res = append(res, md)
			size += uint64(md.Size)
		}
	}
	return res, size
}

// Returns a map from unique 64-bit ids to messages in this folder
func (f *ImapFolderMeta) GetMap() map[uint64]MessageMeta {
	res := make(map[uint64]MessageMeta)
	for _, m := range f.Messages {
		res[m.GetUuid()] = m
	}
	return res
}
