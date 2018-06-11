package imap

import (
	"bytes"
	"time"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
)

// CopyTo copies all specified messages from one directory to another.
// Invalid UIDs are ignored!
func (c *Client) CopyTo(fromDir string, targetDir string, uids ...uint32) error {
	if _, err := c.cl.Select(fromDir, false); err != nil {
		return err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	for _, i := range uids {
		seqset.AddNum(i)
	}

	return c.cl.UidCopy(&seqset, targetDir)
}

// MoveTo copies all messages to specified directory and removes them from
// source directory.
// Invalid UIDs are ignored!
func (c *Client) MoveTo(fromDir string, targetDir string, uids ...uint32) error {
	if _, err := c.cl.Select(fromDir, false); err != nil {
		return err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	if err := c.cl.UidCopy(&seqset, targetDir); err != nil {
		return err
	}

	// c.cl.Close will remove flagged messages.
	return c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{eimap.DeletedFlag}, nil)
}

// Delete deletes all specified messages.
// Invalid UIDs are ignored!
func (c *Client) Delete(dir string, uids ...uint32) error {
	if _, err := c.cl.Select(dir, false); err != nil {
		return err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	return c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{eimap.DeletedFlag}, nil)
}

const (
	TagRead  = eimap.SeenFlag
	TagDraft = eimap.DraftFlag
)

// Tag adds a tag to listed messages.
// Invalid UIDs are ignored!
func (c *Client) Tag(dir string, tag string, uids ...uint32) error {
	if _, err := c.cl.Select(dir, false); err != nil {
		return err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	return c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{tag}, nil)
}

// UnTag removes a tag from listed messages.
// Invalid UIDs are ignored!
func (c *Client) UnTag(dir string, tag string, uids ...uint32) error {
	if _, err := c.cl.Select(dir, false); err != nil {
		return err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	return c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.RemoveFlags, true), []interface{}{tag}, nil)
}

// Create creates new message in specified directory, flags and date are option
// and can be null.
func (c *Client) Create(dir string, flags []string, date time.Time, msg *common.Msg) error {
	buf := bytes.Buffer{}
	msg.Write(&buf)
	return c.cl.Append(dir, flags, date, &buf)
}

// Replace replaces existing message with different one *in one mailbox* (delete+create).
//
// UID will be changed. Replace with invalid input UID works exactly the
// same as Create because invalid uids are ignored.
//
// This function works a bit differently from delete+create. If message
// creation fails then no message will be deleted.
func (c *Client) Replace(dir string, uid uint32, flags []string, date time.Time, msg *common.Msg) error {
	if _, err := c.cl.Select(dir, false); err != nil {
		return err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)

	// Mark old message as deleted.
	if err := c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{eimap.DeletedFlag}, nil); err != nil {
		return err
	}

	// Create new message.
	buf := bytes.Buffer{}
	msg.Write(&buf)
	if err := c.cl.Append(dir, flags, date, &buf); err != nil {
		// Message creation failed. Abort old message deletion.
		if err := c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.RemoveFlags, true), []interface{}{eimap.DeletedFlag}, nil); err != nil {
			return err
		}
		return err
	}

	return nil
}
