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

// Metadata for an email message on IMAP server or in a local .mbox file
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
func filterNewMsgMetaData(mds []MessageMeta, lookup map[uint64]MessageMeta) (res []MessageMeta, size uint64) {
	res = []MessageMeta{}
	size = 0
	for _, md := range mds {
		if _, ok := lookup[md.GetUuid()]; !ok {
			res = append(res, md)
			size += uint64(md.Size)
		}
	}
	return res, size
}
