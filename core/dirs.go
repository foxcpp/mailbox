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
		Logger.Printf("CreateSubdir failed (%v, %v, %v): %v\n", accountId, parentDir, newDir, err)
	} else {
		c.caches[accountId].AddDir(c.joinWithParentDir(parentDir, newDir))
	}
	return err
}

func (c *Client) RemoveDir(accountId, parentDir, dir string) error {
	Logger.Printf("Removing directory (%v, %v, %v)...\n", accountId, parentDir, dir)
	dirName := c.joinWithParentDir(parentDir, dir)

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
		Logger.Printf("CreateSubdir failed (%v, %v, %v): %v\n", accountId, parentDir, dir, err)
	} else {
		c.caches[accountId].RemoveDir(dirName)
	}
	return err
}

func (c *Client) MoveDir(accountId, oldParentDir, newParentDir, dir string) error {
	Logger.Printf("Moving directory (%v, %v from %v to %v)...\n", accountId, dir, oldParentDir, newParentDir)
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
		Logger.Printf("MoveDir failed (%v, %v from %v to %v): %v\n", accountId, dir, oldParentDir, newParentDir, err)
	} else {
		c.caches[accountId].RenameDir(fromNorm, c.joinWithParentDir(newParentDir, dir))
	}
	return err
}

func (c *Client) RenameDir(accountId, oldName, newName string) error {
	Logger.Printf("Renaming directory (%v, from %v to %v)...\n", accountId, oldName, newName)

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
		Logger.Printf("RenameDir failed (%v, from %v to %v): %v\n", accountId, oldName, newName, err)
	} else {
		c.caches[accountId].RenameDir(oldName, newName)
	}
	return err
}
