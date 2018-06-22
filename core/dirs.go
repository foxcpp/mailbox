package core

func (c *Client) joinWithParentDir(parentDir, childDir string) string {
	if parentDir == "" {
		return childDir
	}
	return parentDir + "|" + childDir
}

// CreateDir... creates directories!
// Note: To create directory with root as parent pass "" as parentDir.
func (c *Client) CreateDir(accountId, parentDir, newDir string) error {
	Logger.Printf("Creating directory (%v, %v, %v)...\n", accountId, parentDir, newDir)
	err := c.imapConns[accountId].CreateDir(c.rawDirName(c.joinWithParentDir(parentDir, newDir)))
	if err != nil {
		Logger.Printf("CreateSubdir failed (%v, %v, %v): %v\n", accountId, parentDir, newDir, err)
	} else {
		c.caches[accountId].lock.Lock()
		c.caches[accountId].dirs.Add(c.joinWithParentDir(parentDir, newDir))
		c.caches[accountId].dirty = true
		c.caches[accountId].lock.Unlock()
	}
	return err
}

func (c *Client) RemoveDir(accountId, parentDir, dir string) error {
	Logger.Printf("Removing directory (%v, %v, %v)...\n", accountId, parentDir, dir)
	dirName := c.joinWithParentDir(parentDir, dir)
	err := c.imapConns[accountId].RemoveDir(c.rawDirName(dirName))
	if err != nil {
		Logger.Printf("CreateSubdir failed (%v, %v, %v): %v\n", accountId, parentDir, dir, err)
	} else {
		c.caches[accountId].lock.Lock()
		c.caches[accountId].dirs.Remove(dirName)
		c.caches[accountId].dirty = true
		c.caches[accountId].lock.Unlock()
	}
	return err
}

func (c *Client) MoveDir(accountId, oldParentDir, newParentDir, dir string) error {
	Logger.Printf("Moving directory (%v, %v from %v to %v)...\n", accountId, dir, oldParentDir, newParentDir)
	fromNorm := c.joinWithParentDir(oldParentDir, dir)
	fromRaw := c.rawDirName(fromNorm)
	toRaw := c.rawDirName(c.joinWithParentDir(newParentDir, dir))

	err := c.imapConns[accountId].RenameDir(fromRaw, toRaw)
	if err != nil {
		Logger.Printf("MoveDir failed (%v, %v from %v to %v): %v\n", accountId, dir, oldParentDir, newParentDir, err)
	} else {
		c.caches[accountId].lock.Lock()
		c.caches[accountId].dirs.Remove(fromNorm)
		c.caches[accountId].dirs.Add(c.joinWithParentDir(newParentDir, dir))
		c.caches[accountId].dirty = true
		c.caches[accountId].lock.Unlock()
	}
	return err
}

func (c *Client) RenameDir(accountId, oldName, newName string) error {
	Logger.Printf("Renaming directory (%v, from %v to %v)...\n", accountId, oldName, newName)

	err := c.imapConns[accountId].RenameDir(c.rawDirName(oldName), c.rawDirName(newName))
	if err != nil {
		Logger.Printf("RenameDir failed (%v, from %v to %v): %v\n", accountId, oldName, newName, err)
	} else {
		c.caches[accountId].lock.Lock()
		c.caches[accountId].dirs.Remove(oldName)
		c.caches[accountId].dirs.Add(newName)
		c.caches[accountId].dirty = true
		c.caches[accountId].lock.Unlock()
	}
	return err
}
