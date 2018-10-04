package imap

import (
	"bytes"
	"fmt"
	"time"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
)

type TooBigError struct {
	MaxSizeBytes uint
}

func (e TooBigError) Error() string {
	return fmt.Sprintf("server doesn't accepts messages bigger than %v bytes", e.MaxSizeBytes)
}

// CopyTo copies all specified messages from one directory to another.
//
// Invalid UIDs are ignored. Invalid dir names lead to error.
func (c *Client) CopyTo(fromDir string, targetDir string, uids ...uint32) error {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	if _, err := c.ensureSelected(fromDir, false); err != nil {
		return err
	}
	defer c.cl.Expunge(nil)

	seqset := eimap.SeqSet{}
	for _, i := range uids {
		seqset.AddNum(i)
	}

	return c.cl.UidCopy(&seqset, targetDir)
}

// MoveTo copies all messages to specified directory and removes them from
// source directory.
//
// Invalid UIDs are ignored. Invalid dir names lead to error.
func (c *Client) MoveTo(fromDir string, targetDir string, uids ...uint32) error {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	if _, err := c.ensureSelected(fromDir, false); err != nil {
		return err
	}

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	return c.move.UidMoveWithFallback(&seqset, targetDir)
}

// Delete deletes all specified messages.
//
// Invalid UIDs are ignored. Invalid dir names lead to error.
func (c *Client) Delete(dir string, uids ...uint32) error {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	if _, err := c.ensureSelected(dir, false); err != nil {
		return err
	}

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
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	if _, err := c.ensureSelected(dir, false); err != nil {
		return err
	}

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	return c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{tag}, nil)
}

// UnTag removes a tag from listed messages.
// Invalid UIDs are ignored!
func (c *Client) UnTag(dir string, tag string, uids ...uint32) error {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	if _, err := c.ensureSelected(dir, false); err != nil {
		return err
	}

	seqset := eimap.SeqSet{}
	seqset.AddNum(uids...)

	return c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.RemoveFlags, true), []interface{}{tag}, nil)
}

// Create creates new message in specified directory, flags and date are optional
// and can be null.
func (c *Client) Create(dir string, flags []string, date time.Time, msg *common.Msg) (uint32, error) {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	status, err := c.ensureSelected(dir, false)
	if err != nil {
		return 0, err
	}

	buf := bytes.Buffer{}
	msg.Write(&buf)

	uidplus, err := c.uidplus.SupportUidPlus()
	if err != nil {
		return 0, err
	}
	if uidplus {
		_, uid, err := c.uidplus.Append(dir, flags, date, &buf)
		return uid, err
	} else {
		// This may not work correctly and break a lot of things but we can't
		// do anything better.
		// See https://tools.ietf.org/html/rfc3501#section-2.3.1.1
		return status.UidNext, c.cl.Append(dir, flags, date, &buf)
	}
}

// Replace replaces existing message with different one *in one mailbox*
// (delete+create).
//
// UID will be changed and new one returned. Replace with invalid input UID works
// exactly the same as Create because invalid UIDs are ignored.
//
// This function works a bit differently from delete+create. If message
// creation fails then no message will be deleted.
func (c *Client) Replace(dir string, uid uint32, flags []string, date time.Time, msg *common.Msg) (uint32, error) {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	c.stopIdle()
	defer c.resumeIdle()

	status, err := c.cl.Select(dir, false)
	if err != nil {
		return 0, err
	}
	defer c.cl.Expunge(nil)

	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)

	if err := c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{eimap.DeletedFlag}, nil); err != nil {
		return 0, err
	}

	// Create new message.
	buf := bytes.Buffer{}
	msg.Write(&buf)

	uidplus, err := c.uidplus.SupportUidPlus()
	if err != nil {
		return 0, err
	}
	var nuid uint32
	if uidplus {
		_, nuid, err = c.uidplus.Append(dir, flags, date, &buf)
	} else {
		err = c.cl.Append(dir, flags, date, &buf)
		// This may not work correctly and break a lot of things but we can't
		// do anything better.
		// See https://tools.ietf.org/html/rfc3501#section-2.3.1.1
		nuid = status.UidNext
	}
	if err != nil {
		return 0, err
	}

	// Mark old version as deleted.
	//
	// We don't do it before message creation because otherwise it will lead to
	// lost message if connection is lost after deletion:
	// 1. We mark old message as deleted
	// 2. We try to create new message
	// 3. We loss connection and reconnect
	// 4. We (or somebody else) selects and desselects this directory.
	// 5. Old message is deleted because it was marked as deleted. New
	//    one is not created. User is angry.
	//
	// With message delete after creation of new one this will lead to only duplicate,
	// but that's better  than to loss both versions.
	if err := c.cl.UidStore(&seqset, eimap.FormatFlagsOp(eimap.AddFlags, true), []interface{}{eimap.DeletedFlag}, nil); err != nil {
		return 0, err
	}

	return nuid, err
}
