package core

import (
	"errors"
	"strings"
)

/*
Normalized directory name - name stored in cache and all other client structures. Separator is | (vertical line).
Raw directory name - name passed to/received from server. Separator is server-defined.
*/

const NormalizedSeparator = "|"

func (c *Client) normalizeDirName(accountId, raw string) string {
	return strings.Replace(raw, c.dirSep(accountId), NormalizedSeparator, -1)
}

func (c *Client) rawDirName(accountId, normalized string) string {
	return strings.Replace(normalized, NormalizedSeparator, c.dirSep(accountId), -1)
}

func (c *Client) splitDirName(name string) []string {
	return strings.Split(name, NormalizedSeparator)
}

func (c *Client) joinDirName(parts []string) string {
	return strings.Join(parts, NormalizedSeparator)
}

func (c *Client) joinWithParentDir(parentDir, childDir string) string {
	if parentDir == "" {
		return childDir
	}
	return parentDir + NormalizedSeparator + childDir
}

// CreateDir... creates directories!
//
// parentDir is created (along with all parents) if it doesn't exists.
//
// To create directory with root as parent pass "" as parentDir.
// Invalid account ID will lead to error.
func (c *Client) CreateDir(accountId, parentDir, newDir string) error {
	if strings.Contains(newDir, "|") {
		return errors.New("create dir: dir name may not contain |")
	}

	c.debugLog.Printf("Creating directory (%v, %v, %v)...\n", accountId, parentDir, newDir)

	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		err = c.imapConns[accountId].CreateDir(c.rawDirName(accountId, c.joinWithParentDir(parentDir, newDir)))
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return err
		}
	}
	if err != nil {
		c.debugLog.Printf("CreateSubdir failed (%v, %v, %v): %v\n", accountId, parentDir, newDir, err)
	} else {
		// Add all parents to cache (AddDir no-op if dirs already exist).
		splittenDirname := c.splitDirName(parentDir)
		for len(splittenDirname) != 0 {
			c.caches[accountId].AddDir(c.joinDirName(splittenDirname))
			splittenDirname = splittenDirname[:len(splittenDirname)-1]
		}

		c.caches[accountId].AddDir(c.joinWithParentDir(parentDir, newDir))
	}
	return err
}

// RemoveDir removes directory.
//
// It's not possible to remove INBOX.
//
// To remove directory from tree root pass "" as parentDir.
// Invalid account ID will lead to error.
func (c *Client) RemoveDir(accountId, parentDir, dir string) error {
	// TODO: Remove children directories.

	c.debugLog.Printf("Removing directory (%v, %v, %v)...\n", accountId, parentDir, dir)
	dirName := c.joinWithParentDir(parentDir, dir)

	if dirName == "INBOX" {
		return errors.New("remove dir: can't remove INBOX")
	}

	var err error

	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		err = c.imapConns[accountId].RemoveDir(c.rawDirName(accountId, dirName))
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return err
		}
	}

	if err != nil {
		c.debugLog.Printf("RemoveSubdir failed (%v, %v, %v): %v\n", accountId, parentDir, dir, err)
	} else {
		c.caches[accountId].RemoveDir(dirName)
	}
	return err
}

// MoveDir moves directory from one tree part to another.
//
// All dir's childrens also moved.
// If oldParentDir == newParentDir behavior is equivalent to RenameDir.
//
// It's recommended to temporary block UI during this operation to avoid race
// conditions (user trying to interact using old directory name while moving
// is in progress).
func (c *Client) MoveDir(accountId, oldParentDir, newParentDir, dir string) error {
	c.debugLog.Printf("Moving directory (%v, %v from %v to %v)...\n", accountId, dir, oldParentDir, newParentDir)
	fromNorm := c.joinWithParentDir(oldParentDir, dir)
	fromRaw := c.rawDirName(accountId, fromNorm)
	toRaw := c.rawDirName(accountId, c.joinWithParentDir(newParentDir, dir))

	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		err = c.imapConns[accountId].RenameDir(fromRaw, toRaw)
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return err
		}
	}

	if err != nil {
		c.debugLog.Printf("MoveDir failed (%v, %v from %v to %v): %v\n", accountId, dir, oldParentDir, newParentDir, err)
	} else {
		c.caches[accountId].RenameDir(fromNorm, c.joinWithParentDir(newParentDir, dir))
	}
	return err
}

// RenameDir changes name of directory from oldName and newName.
//
// It's recommended to temporary block UI during this operation to avoid race
// conditions (user trying to interact using old directory name while renaming
// is in progress).
func (c *Client) RenameDir(accountId, oldName, newName string) error {
	if strings.Contains(newName, "|") {
		return errors.New("rename dir: dir name may not contain |")
	}

	c.debugLog.Printf("Renaming directory (%v, from %v to %v)...\n", accountId, oldName, newName)

	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		err = c.imapConns[accountId].RenameDir(c.rawDirName(accountId, oldName), c.rawDirName(accountId, newName))
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return err
		}
	}

	if err != nil {
		c.debugLog.Printf("RenameDir failed (%v, from %v to %v): %v\n", accountId, oldName, newName, err)
	} else {
		c.caches[accountId].RenameDir(oldName, newName)
	}
	return err
}
